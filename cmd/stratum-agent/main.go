package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/agent/serverjar"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if _, ok := err.(usageError); ok {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stratum-agent",
		Short:         "Run Stratum Agent process supervision endpoints",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newServeCommand())
	return cmd
}

func newServeCommand() *cobra.Command {
	var listen string
	var token string
	var runtimeMode string
	var runtimeRoot string
	var runtimeProfiles string
	var httpProxy string
	var controllerURL string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Stratum Agent HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(listen, token, runtimeMode, runtimeRoot, runtimeProfiles, httpProxy, controllerURL)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8787", "listen address")
	cmd.Flags().StringVar(&token, "token", os.Getenv("STRATUM_AGENT_TOKEN"), "optional bearer token")
	cmd.Flags().StringVar(&runtimeMode, "runtime-mode", "process", "runtime mode: dummy-process, process, or mcdr")
	cmd.Flags().StringVar(&runtimeRoot, "runtime-root", ".stratum/runtime", "trusted runtime working root")
	cmd.Flags().StringVar(&runtimeProfiles, "runtime-profiles", "", "trusted local RuntimeProfile JSON configuration")
	cmd.Flags().StringVar(&httpProxy, "http-proxy", os.Getenv("STRATUM_HTTP_PROXY"), "HTTP proxy for downloads (e.g., http://127.0.0.1:10808)")
	cmd.Flags().StringVar(&controllerURL, "controller-url", os.Getenv("STRATUM_CONTROLLER_URL"), "Controller URL for agent registration")
	return cmd
}

func serve(listen, token, runtimeMode, runtimeRoot, runtimeProfiles, httpProxy, controllerURL string) error {
	switch runtimeMode {
	case "dummy-process", "process", "mcdr":
	default:
		return usageError{err: fmt.Errorf("unsupported runtime mode %q; supported: dummy-process, process, mcdr", runtimeMode)}
	}

	if httpProxy != "" {
		if err := setHTTPProxy(httpProxy); err != nil {
			return fmt.Errorf("set HTTP proxy: %w", err)
		}
		if err := os.Setenv("STRATUM_HTTP_PROXY", httpProxy); err != nil {
			return fmt.Errorf("set STRATUM_HTTP_PROXY: %w", err)
		}
		log.Printf("HTTP proxy configured: %s", httpProxy)
	}

	logger := log.New(os.Stderr, "stratum-agent ", log.LstdFlags)

	serverjar.DefaultVersionCache().Start()
	logger.Printf("started Minecraft version poller (interval=6h)")
	if runtimeProfiles == "-" {
		runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, runtimeprofile.Builtins(), runtimeRoot)
		if err != nil {
			return err
		}
		server := httptransport.NewServer(runtimeAgent, token, logger)
		logger.Printf("listening on %s with %s supervision (auth=%t)", listen, runtimeMode, token != "")
		return serveHTTP(listen, server.Handler())
	}

	registry := runtimeprofile.Builtins()
	if runtimeProfiles != "" {
		profiles, err := runtimeprofile.LoadTrustedFile(runtimeProfiles)
		if err != nil {
			return err
		}
		if err := registry.RegisterAll(profiles); err != nil {
			return fmt.Errorf("register runtime profiles from %q: %w", runtimeProfiles, err)
		}
		profileDir := filepath.Dir(runtimeProfiles)
		watcher := runtimeprofile.NewWatcher(registry, profileDir, 30*time.Second)
		watcher.Start()
		defer watcher.Stop()
		logger.Printf("watching runtime profile directory: %s", profileDir)
	}
	runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, registry, runtimeRoot)
	if err != nil {
		return err
	}

	if controllerURL != "" {
		if err := registerWithController(controllerURL, token, listen, runtimeMode, logger); err != nil {
			logger.Printf("controller registration failed: %v", err)
		}
	}

	server := httptransport.NewServer(runtimeAgent, token, logger)
	logger.Printf("listening on %s with %s supervision (auth=%t)", listen, runtimeMode, token != "")
	if err := serveHTTP(listen, server.Handler()); err != nil {
		return err
	}
	return nil
}

func serveHTTP(listen string, handler http.Handler) error {
	server := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

func setHTTPProxy(proxyURL string) error {
	return serverjar.SetProxy(proxyURL)
}

func registerWithController(controllerURL, token, listen, runtimeMode string, logger *log.Logger) error {
	controllerURL = strings.TrimSuffix(controllerURL, "/")
	client := &http.Client{Timeout: 30 * time.Second}
	registerURL := controllerURL + "/v1/agents/register"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, registerURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Agent-Listen", listen)
	req.Header.Set("X-Agent-Mode", runtimeMode)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("controller returned HTTP %d", resp.StatusCode)
	}
	logger.Printf("registered with controller at %s (agent listen=%s)", controllerURL, listen)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			heartURL := controllerURL + "/v1/agents/heartbeat"
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, heartURL, nil)
			if err != nil {
				cancel()
				logger.Printf("controller heartbeat request failed: %v", err)
				continue
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			req.Header.Set("X-Agent-Listen", listen)
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
			} else {
				logger.Printf("controller heartbeat failed: %v", err)
			}
			cancel()
		}
	}()
	return nil
}
