package session

import "testing"

func TestTransition(t *testing.T) {
	s := Session{ID: "session-1", State: StateCreated}
	for _, state := range []State{StatePreparing, StateStarting, StateRunning, StateStopping, StateStopped, StateArchived, StateDeleted} {
		if err := s.Transition(state); err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
}

func TestFreezeAndCrashTransitions(t *testing.T) {
	if !CanTransition(StateRunning, StateFrozen) || !CanTransition(StateFrozen, StateRunning) {
		t.Fatal("running and frozen should transition in both directions")
	}
	if !CanTransition(StateRunning, StateCrashed) || !CanTransition(StateStarting, StateCrashed) {
		t.Fatal("running and starting should be able to crash")
	}
	if CanTransition(StateStopped, StateFrozen) || CanTransition(StateCreated, StateArchived) {
		t.Fatal("unsupported lifecycle shortcut was allowed")
	}
}

func TestTransitionRejectsInvalidChange(t *testing.T) {
	s := Session{ID: "session-1", State: StateCreated}
	if err := s.Transition(StateRunning); err == nil {
		t.Fatal("expected created to running transition to fail")
	}
	if s.State != StateCreated {
		t.Fatalf("invalid transition changed state to %s", s.State)
	}
}
