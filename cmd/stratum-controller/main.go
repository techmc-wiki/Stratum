package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stratum-controller",
		Short:         "Run Stratum Controller services",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q", args[0])
			}
			fmt.Fprintln(cmd.OutOrStdout(), "stratum-controller MVP stub: HTTP API and persistence are not started yet")
			// TODO: load configuration, repositories, services, and the HTTP API.
			return nil
		},
	}
	return cmd
}
