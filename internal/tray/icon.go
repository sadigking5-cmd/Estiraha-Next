// Package tray manages Windows System Tray integration, deterministic programmatic icon
// generation, window hide/restore state tracking, and clean shutdown teardown.
package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

// GenerateAppIconPNG produces a valid, deterministic 64x64 RGBA PNG icon in memory
// (cinematic obsidian background + warm gold play/cinema emblem) with zero external file
// dependencies and zero SVG usage.
func GenerateAppIconPNG() []byte {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.RGBA{R: 15, G: 23, B: 42, A: 255}     // Deep slate-obsidian (#0F172A)
	border := color.RGBA{R: 212, G: 175, B: 55, A: 255} // Cinematic gold (#D4AF37)
	accent := color.RGBA{R: 245, G: 197, B: 66, A: 255} // Bright warm gold

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if x < 3 || x >= size-3 || y < 3 || y >= size-3 {
				img.SetRGBA(x, y, border)
			} else {
				img.SetRGBA(x, y, bg)
			}
		}
	}

	// Draw centered play triangle emblem
	for y := 18; y <= 46; y++ {
		halfSpan := 14 - abs(y-32)
		if halfSpan < 0 {
			halfSpan = 0
		}
		maxX := 24 + halfSpan*2
		for x := 24; x <= maxX && x < size-8; x++ {
			img.SetRGBA(x, y, accent)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// StateTracker tracks System Tray lifecycle and window visibility transitions safely across goroutines.
type StateTracker struct {
	mu            sync.RWMutex
	initialized   bool
	destroyed     bool
	windowVisible bool
	hideCount     int
	restoreCount  int
}

// Snapshot represents an immutable read of System Tray state for UI & automated smoke checks.
type Snapshot struct {
	Initialized   bool `json:"initialized"`
	Destroyed     bool `json:"destroyed"`
	WindowVisible bool `json:"windowVisible"`
	HideCount     int  `json:"hideCount"`
	RestoreCount  int  `json:"restoreCount"`
}

func NewStateTracker() *StateTracker {
	return &StateTracker{windowVisible: true}
}

func (s *StateTracker) MarkInitialized() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialized = true
	s.destroyed = false
}

func (s *StateTracker) RecordHide() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowVisible = false
	s.hideCount++
}

func (s *StateTracker) RecordRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowVisible = true
	s.restoreCount++
}

func (s *StateTracker) MarkDestroyed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed = true
}

func (s *StateTracker) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Initialized:   s.initialized,
		Destroyed:     s.destroyed,
		WindowVisible: s.windowVisible,
		HideCount:     s.hideCount,
		RestoreCount:  s.restoreCount,
	}
}
