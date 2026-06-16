package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newRoomsCommand() *cobra.Command {
	cmd := groupCommand("rooms", "Manage rooms")
	cmd.AddCommand(storeCommand("create", "Create a room", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createRoom(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("list", "List rooms", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listRooms(ctx, runtime.store, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
