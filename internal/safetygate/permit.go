// Package safetygate defines the acyclic user-approval ExecutionPermit contract
// shared across review, fsops, errorsbox, and organizer without circular imports.
package safetygate

import (
	"errors"
	"time"
)

var (
	ErrUnapprovedExecution  = errors.New("review safety lock: cannot execute operation without explicit user approval")
	ErrEmptyBatchOrItem     = errors.New("review safety lock: batch_id and review_item_id are required")
	ErrTargetCollisionAbort = errors.New("review safety lock: destination file already exists; overwriting is strictly prohibited")
)

// ApprovalDecision represents the user's explicit verdict on a proposed review item.
type ApprovalDecision string

const (
	DecisionPending  ApprovalDecision = "pending"
	DecisionApproved ApprovalDecision = "approved"
	DecisionExcluded ApprovalDecision = "excluded"
	DecisionRejected ApprovalDecision = "rejected"
	DecisionDeferred ApprovalDecision = "deferred"
)

// ExecutionPermit proves that a specific review item was explicitly approved by the user.
type ExecutionPermit struct {
	BatchID      string
	ReviewItemID string
	Decision     ApprovalDecision
	ApprovedAt   time.Time
}

// ValidatePermit verifies that a proposed filesystem operation has passed explicit user review.
func ValidatePermit(p ExecutionPermit) error {
	if p.BatchID == "" || p.ReviewItemID == "" {
		return ErrEmptyBatchOrItem
	}
	if p.Decision != DecisionApproved || p.ApprovedAt.IsZero() {
		return ErrUnapprovedExecution
	}
	return nil
}
