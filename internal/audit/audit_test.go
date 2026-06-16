package audit

import (
	"testing"
	"time"
)

func TestNewAuditEvent(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	event, err := NewEvent("event-1", "user-1", "session.create", "session", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if event.CreatedAt != now || event.Action != "session.create" {
		t.Fatalf("unexpected event: %+v", event)
	}
}
