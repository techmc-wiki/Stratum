package resourcepolicy

import (
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/session"
)

type Policy struct {
	ID                 string         `json:"id"`
	GlobalMaxRunning   int            `json:"globalMaxRunning"`
	PerProjectMax      int            `json:"perProjectMax"`
	PerUserMax         int            `json:"perUserMax"`
	ReviewMaxRunning   int            `json:"reviewMaxRunning"`
	SessionIdleTimeout time.Duration  `json:"sessionIdleTimeout"`
	TemporaryTTL       time.Duration  `json:"temporaryTtl"`
	AllowedTypes       []session.Type `json:"allowedTypes"`
}

type Usage struct {
	GlobalRunning  int
	ProjectRunning int
	UserRunning    int
	ReviewRunning  int
}

type DenialReason string

const (
	DeniedTypeNotAllowed DenialReason = "session_type_not_allowed"
	DeniedGlobalLimit    DenialReason = "global_limit_reached"
	DeniedProjectLimit   DenialReason = "project_limit_reached"
	DeniedUserLimit      DenialReason = "user_limit_reached"
	DeniedReviewLimit    DenialReason = "review_limit_reached"
)

type Decision struct {
	Allowed bool         `json:"allowed"`
	Reason  DenialReason `json:"reason,omitempty"`
	Message string       `json:"message"`
}

func MVPDefault() Policy {
	return Policy{
		ID: "default", GlobalMaxRunning: 8, PerProjectMax: 4, PerUserMax: 2,
		ReviewMaxRunning: 1, SessionIdleTimeout: 30 * time.Minute, TemporaryTTL: 4 * time.Hour,
		AllowedTypes: []session.Type{session.TypeShared, session.TypeFork, session.TypePrivate, session.TypeReview},
	}
}

func Evaluate(policy Policy, usage Usage, requested session.Type) Decision {
	if !contains(policy.AllowedTypes, requested) {
		return deny(DeniedTypeNotAllowed, "requested session type is not enabled by policy")
	}
	if policy.GlobalMaxRunning > 0 && usage.GlobalRunning >= policy.GlobalMaxRunning {
		return deny(DeniedGlobalLimit, fmt.Sprintf("global running session limit of %d reached", policy.GlobalMaxRunning))
	}
	if policy.PerProjectMax > 0 && usage.ProjectRunning >= policy.PerProjectMax {
		return deny(DeniedProjectLimit, fmt.Sprintf("project running session limit of %d reached", policy.PerProjectMax))
	}
	if (requested == session.TypePrivate || requested == session.TypeFork) && policy.PerUserMax > 0 && usage.UserRunning >= policy.PerUserMax {
		return deny(DeniedUserLimit, fmt.Sprintf("user running session limit of %d reached", policy.PerUserMax))
	}
	if requested == session.TypeReview && policy.ReviewMaxRunning > 0 && usage.ReviewRunning >= policy.ReviewMaxRunning {
		return deny(DeniedReviewLimit, fmt.Sprintf("review session limit of %d reached", policy.ReviewMaxRunning))
	}
	return Decision{Allowed: true, Message: "session request is within resource policy"}
}

func deny(reason DenialReason, message string) Decision {
	return Decision{Reason: reason, Message: message}
}

func contains(types []session.Type, requested session.Type) bool {
	for _, candidate := range types {
		if candidate == requested {
			return true
		}
	}
	return false
}
