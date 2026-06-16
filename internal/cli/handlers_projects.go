package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func createProject(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("projects create", stderr)
	id := flags.String("id", "", "project ID")
	name := flags.String("name", "", "project name")
	description := flags.String("description", "", "project description")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *name == "" {
		fmt.Fprintln(stderr, "--id and --name are required")
		return 2
	}
	value := project.Project{ID: *id, Name: *name, Description: *description, Members: []project.Member{}, CreatedAt: time.Now().UTC()}
	if err := store.CreateProject(ctx, value); err != nil {
		return reportError(stderr, "create project", err)
	}
	fmt.Fprintf(stdout, "Created project %s (%s).\n", value.ID, value.Name)
	return 0
}

func listProjects(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListProjects(ctx)
	if err != nil {
		return reportError(stderr, "list projects", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\n", value.ID, value.Name)
	}
	return 0
}
