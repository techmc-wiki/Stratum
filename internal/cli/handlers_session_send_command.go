package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
)

func sessionSendCommand(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions send-command", stderr)
	sessionID := flags.String("session", "", "session ID")
	command := flags.String("command", "", "command to send to session runtime stdin")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || strings.TrimSpace(*command) == "" {
		fmt.Fprintln(stderr, "sessions send-command: --session and --command are required")
		return 2
	}
	result, err := agentClient.SendCommand(ctx, *sessionID, *command)
	if err != nil {
		return reportError(stderr, "send command", err)
	}
	fmt.Fprintf(stdout, "Command sent to session %s: status=%s message=%s agent=%s\n", *sessionID, result.Status, result.Message, result.AgentID)
	return 0
}
