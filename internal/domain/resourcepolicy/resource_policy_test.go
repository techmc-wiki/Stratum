package resourcepolicy

import (
	"testing"

	"github.com/stratummc/stratum/internal/domain/session"
)

func TestEvaluateResourcePolicy(t *testing.T) {
	policy := Policy{GlobalMaxRunning: 4, PerProjectMax: 3, PerUserMax: 1, ReviewMaxRunning: 1, AllowedTypes: []session.Type{session.TypeShared, session.TypeFork, session.TypeReview}}

	if decision := Evaluate(policy, Usage{}, session.TypeFork); !decision.Allowed {
		t.Fatalf("expected request to be allowed: %+v", decision)
	}
	if decision := Evaluate(policy, Usage{UserRunning: 1}, session.TypeFork); decision.Reason != DeniedUserLimit {
		t.Fatalf("reason = %q, want %q", decision.Reason, DeniedUserLimit)
	}
	if decision := Evaluate(policy, Usage{}, session.TypePrivate); decision.Reason != DeniedTypeNotAllowed {
		t.Fatalf("reason = %q, want %q", decision.Reason, DeniedTypeNotAllowed)
	}
}
