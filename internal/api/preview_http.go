package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
)

type rpcCallRequest struct {
	Args []json.RawMessage `json:"args"`
}

type rpcCallResponse struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// NewUIPreviewHandler returns an http.Handler serving the embedded production React bundle (`distFS`)
// alongside `/api/rpc/library/<Method>` and `/api/rpc/poc/<Method>` reflection dispatchers.
// Used exclusively when `--ui-preview-addr` is explicitly passed on the CLI for headless UI testing.
func NewUIPreviewHandler(distFS fs.FS, libSvc *LibraryService, pocSvc *PoCService) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/rpc/library/", func(w http.ResponseWriter, r *http.Request) {
		methodName := strings.TrimPrefix(r.URL.Path, "/api/rpc/library/")
		dispatchRPC(w, r, libSvc, methodName)
	})

	mux.HandleFunc("/api/rpc/poc/", func(w http.ResponseWriter, r *http.Request) {
		methodName := strings.TrimPrefix(r.URL.Path, "/api/rpc/poc/")
		dispatchRPC(w, r, pocSvc, methodName)
	})

	fileServer := http.FileServer(http.FS(distFS))
	mux.Handle("/", fileServer)
	return mux
}

func dispatchRPC(w http.ResponseWriter, r *http.Request, target any, methodName string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: "POST required"})
		return
	}
	if target == nil || methodName == "" {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: "service or method not found"})
		return
	}

	mv := reflect.ValueOf(target).MethodByName(methodName)
	if !mv.IsValid() {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: fmt.Sprintf("unknown method %s", methodName)})
		return
	}

	var req rpcCallRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: err.Error()})
			return
		}
	}

	mt := mv.Type()
	numIn := mt.NumIn()
	callArgs := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		paramType := mt.In(i)
		ptr := reflect.New(paramType)
		if i < len(req.Args) && len(req.Args[i]) > 0 {
			if err := json.Unmarshal(req.Args[i], ptr.Interface()); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: fmt.Sprintf("arg %d decode error: %v", i, err)})
				return
			}
		}
		callArgs[i] = ptr.Elem()
	}

	outs := mv.Call(callArgs)
	var result any
	var callErr error
	errIface := reflect.TypeOf((*error)(nil)).Elem()

	for _, out := range outs {
		if out.Type().Implements(errIface) {
			if !out.IsNil() {
				callErr = out.Interface().(error)
			}
		} else {
			result = out.Interface()
		}
	}

	if callErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(rpcCallResponse{Error: callErr.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(rpcCallResponse{Result: result})
}
