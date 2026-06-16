package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func listOperations(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("operations list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	status := flags.String("status", "", "filter by operation status")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	values, err := store.ListOperations(ctx)
	if err != nil {
		return reportError(stderr, "list operations", err)
	}
	for _, value := range values {
		if *sessionID != "" && value.SessionID != *sessionID {
			continue
		}
		if *status != "" && string(value.Status) != *status {
			continue
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.Action, value.Status, value.RequestID)
	}
	return 0
}

func inspectOperation(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("operations inspect", stderr)
	id := flags.String("id", "", "operation ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetOperation(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect operation", err)
	}
	fmt.Fprintf(stdout, "id=%s request=%s actor=%s action=%s session=%s status=%s previous=%s intended=%s final=%s result=%s runtimeProfile=%s errorCode=%s error=%q\n", value.ID, value.RequestID, value.ActorID, value.Action, value.SessionID, value.Status, value.PreviousState, value.IntendedState, value.FinalState, value.Result, value.Metadata["runtimeProfileId"], value.ErrorCode, value.ErrorMessage)
	return 0
}
