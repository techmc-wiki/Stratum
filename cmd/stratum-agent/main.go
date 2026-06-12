package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: stratum-agent serve [--listen 127.0.0.1:8787] [--token TOKEN] [--runtime-mode dummy-process] [--runtime-root PATH] [--runtime-profiles PATH]")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8787", "listen address")
	token := flags.String("token", os.Getenv("STRATUM_AGENT_TOKEN"), "optional bearer token")
	runtimeMode := flags.String("runtime-mode", "dummy-process", "safe runtime mode (dummy-process only)")
	runtimeRoot := flags.String("runtime-root", ".stratum/runtime", "trusted runtime working root")
	runtimeProfiles := flags.String("runtime-profiles", "", "trusted local RuntimeProfile JSON configuration")
	_ = flags.Parse(os.Args[2:])
	if *runtimeMode != "dummy-process" {
		fmt.Fprintf(os.Stderr, "unsupported runtime mode %q; only dummy-process is available\n", *runtimeMode)
		os.Exit(2)
	}

	logger := log.New(os.Stderr, "stratum-agent ", log.LstdFlags)
	registry := runtimeprofile.Builtins()
	if *runtimeProfiles != "" {
		profiles, err := runtimeprofile.LoadTrustedFile(*runtimeProfiles)
		if err != nil {
			logger.Fatal(err)
		}
		if err := registry.RegisterAll(profiles); err != nil {
			logger.Fatalf("register runtime profiles from %q: %v", *runtimeProfiles, err)
		}
	}
	runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, registry, *runtimeRoot)
	if err != nil {
		logger.Fatal(err)
	}
	server := httptransport.NewServer(runtimeAgent, *token, logger)
	logger.Printf("listening on %s with %s supervision (auth=%t)", *listen, *runtimeMode, *token != "")
	if err := http.ListenAndServe(*listen, server.Handler()); err != nil {
		logger.Fatal(err)
	}
}
