package main

import (
	"os"

	"github.com/mclucy/lucy/cmd"
	"github.com/mclucy/lucy/logger"
)

func main() {
	defer logger.DumpHistory()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
