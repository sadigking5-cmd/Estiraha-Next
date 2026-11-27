// Package review implements the Review Center & User Approval Gate.
// Mandatory Law: No file rename, move, error-relocation, or deletion may execute
// unless bound to an explicitly user-approved ExecutionPermit.
package review

import "github.com/sadigking5-cmd/Estiraha-Next/internal/safetygate"

var (
	ErrUnapprovedExecution  = safetygate.ErrUnapprovedExecution
	ErrEmptyBatchOrItem     = safetygate.ErrEmptyBatchOrItem
	ErrTargetCollisionAbort = safetygate.ErrTargetCollisionAbort
)

type ApprovalDecision = safetygate.ApprovalDecision

const (
	DecisionPending  = safetygate.DecisionPending
	DecisionApproved = safetygate.DecisionApproved
	DecisionExcluded = safetygate.DecisionExcluded
	DecisionRejected = safetygate.DecisionRejected
	DecisionDeferred = safetygate.DecisionDeferred
)

type ExecutionPermit = safetygate.ExecutionPermit

func ValidatePermit(p ExecutionPermit) error {
	return safetygate.ValidatePermit(p)
}
