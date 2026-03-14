package subcommands

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/leefowlercu/sigil/internal/config"
)

const (
	appServerListenStdio = "stdio://"
)

type serveTransport string

const (
	serveTransportStdio     serveTransport = "stdio"
	serveTransportWebSocket serveTransport = "websocket"
)

type serveTarget struct {
	Transport  serveTransport
	ListenAddr string
}

func validateServeListen(listen string) error {
	_, err := parseServeListen(listen)
	return err
}

func parseServeListen(listen string) (serveTarget, error) {
	trimmed := strings.TrimSpace(listen)
	if trimmed == "" {
		return serveTarget{}, fmt.Errorf("invalid --listen value; value cannot be empty")
	}
	if trimmed == appServerListenStdio {
		return serveTarget{Transport: serveTransportStdio}, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return serveTarget{}, fmt.Errorf("invalid --listen value; %w", err)
	}
	if parsed.Scheme != "ws" {
		return serveTarget{}, fmt.Errorf("invalid --listen value; scheme must be stdio or ws")
	}
	if parsed.Host == "" {
		return serveTarget{}, fmt.Errorf("invalid --listen value; websocket listener must include host and port")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return serveTarget{}, fmt.Errorf("invalid --listen value; websocket path must be configured with --websocket-path or app_server.websocket.path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return serveTarget{}, fmt.Errorf("invalid --listen value; websocket listener must not include query fragment or user info")
	}

	return serveTarget{
		Transport:  serveTransportWebSocket,
		ListenAddr: parsed.Host,
	}, nil
}

func validateServeHTTPPath(flagName string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("invalid %s value; path cannot be empty", flagName)
	}
	if !strings.HasPrefix(trimmed, "/") {
		return fmt.Errorf("invalid %s value; path must start with /", flagName)
	}
	return nil
}

func validateServeAllowedOrigins(origins []string) error {
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			return fmt.Errorf("invalid --allowed-origin value; origin cannot be empty")
		}
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return fmt.Errorf("invalid --allowed-origin value %q; %w", trimmed, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid --allowed-origin value %q; origin must include scheme and host", trimmed)
		}
	}
	return nil
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, value)
	}
	return cloned
}

func validateOptionalOutputFile(path string) error {
	if path == "" {
		return nil
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("invalid --output-file value; path cannot be empty")
	}
	return nil
}

func writeOutputFile(target string, content []byte) error {
	expanded, err := config.ExpandPath(target)
	if err != nil {
		return fmt.Errorf("failed to resolve output file; %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory; %w", err)
	}
	if err := os.WriteFile(expanded, content, 0o644); err != nil {
		return fmt.Errorf("failed to write output file; %w", err)
	}
	return nil
}
