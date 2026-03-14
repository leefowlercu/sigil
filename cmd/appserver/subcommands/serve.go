package subcommands

import (
	"fmt"
	"log/slog"

	internalappserver "github.com/leefowlercu/sigil/internal/appserver"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/spf13/cobra"
)

var appServerServeListen string
var appServerServeWebSocketPath string
var appServerServeReadyPath string
var appServerServeLivePath string
var appServerServeAllowedOrigins []string

// NewServeCmd builds the app-server serve command.
func NewServeCmd() *cobra.Command {
	appServerServeListen = appServerListenStdio
	appServerServeWebSocketPath = ""
	appServerServeReadyPath = ""
	appServerServeLivePath = ""
	appServerServeAllowedOrigins = nil

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve Sigil app-server requests over stdio or WebSocket",
		Long: "sigil app-server serve runs the Sigil app-server transport.\n\n" +
			"This command serves the shared JSON-RPC 2.0 protocol over stdio JSONL or a localhost/dev WebSocket listener so clients can exercise handshake, routing, heartbeat, and protocol generation workflows on one transport surface.",
		Example: "# Serve the stdio transport\n" +
			"  sigil app-server serve --listen stdio://\n\n" +
			"# Serve the localhost WebSocket transport\n" +
			"  sigil app-server serve --listen ws://127.0.0.1:8765",
		PreRunE: validateServeInputs,
		RunE:    runServeCommand,
	}

	cmd.Flags().StringVar(&appServerServeListen, "listen", appServerListenStdio, "App-server listener URI")
	cmd.Flags().StringVar(&appServerServeWebSocketPath, "websocket-path", "", "Optional override for app_server.websocket.path")
	cmd.Flags().StringVar(&appServerServeReadyPath, "ready-path", "", "Optional override for app_server.health.ready_path")
	cmd.Flags().StringVar(&appServerServeLivePath, "live-path", "", "Optional override for app_server.health.live_path")
	cmd.Flags().StringSliceVar(&appServerServeAllowedOrigins, "allowed-origin", nil, "Optional repeatable override for app_server.allowed_origins")
	return cmd
}

func validateServeInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}
	target, err := parseServeListen(appServerServeListen)
	if err != nil {
		return err
	}
	if target.Transport == serveTransportWebSocket {
		if cmd.Flags().Changed("websocket-path") {
			if err := validateServeHTTPPath("--websocket-path", appServerServeWebSocketPath); err != nil {
				return err
			}
		}
		if cmd.Flags().Changed("ready-path") {
			if err := validateServeHTTPPath("--ready-path", appServerServeReadyPath); err != nil {
				return err
			}
		}
		if cmd.Flags().Changed("live-path") {
			if err := validateServeHTTPPath("--live-path", appServerServeLivePath); err != nil {
				return err
			}
		}
		if cmd.Flags().Changed("allowed-origin") {
			if err := validateServeAllowedOrigins(appServerServeAllowedOrigins); err != nil {
				return err
			}
		}
	} else if cmd.Flags().Changed("websocket-path") || cmd.Flags().Changed("ready-path") || cmd.Flags().Changed("live-path") || cmd.Flags().Changed("allowed-origin") {
		return fmt.Errorf("websocket-specific flags require --listen ws://HOST:PORT")
	}
	if err := validateServeListen(appServerServeListen); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runServeCommand(cmd *cobra.Command, _ []string) error {
	cfg := config.MustGet()
	server := internalappserver.New(cfg)
	target, err := parseServeListen(appServerServeListen)
	if err != nil {
		return err
	}

	switch target.Transport {
	case serveTransportStdio:
		appServerServeLogger().Info("starting app-server serve command", "listen", appServerServeListen)
		if err := server.ServeStdio(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout()); err != nil {
			return fmt.Errorf("failed to serve app-server; %w", err)
		}
		appServerServeLogger().Info("app-server serve command completed", "listen", appServerServeListen)
		return nil
	case serveTransportWebSocket:
		websocketPath := cfg.AppServer.WebSocket.Path
		if cmd.Flags().Changed("websocket-path") {
			websocketPath = appServerServeWebSocketPath
		}
		readyPath := cfg.AppServer.Health.ReadyPath
		if cmd.Flags().Changed("ready-path") {
			readyPath = appServerServeReadyPath
		}
		livePath := cfg.AppServer.Health.LivePath
		if cmd.Flags().Changed("live-path") {
			livePath = appServerServeLivePath
		}
		allowedOrigins := cfg.AppServer.AllowedOrigins
		if cmd.Flags().Changed("allowed-origin") {
			allowedOrigins = cloneStringSlice(appServerServeAllowedOrigins)
		}

		appServerServeLogger().Info(
			"starting websocket app-server serve command",
			"listen_addr", target.ListenAddr,
			"websocket_path", websocketPath,
			"ready_path", readyPath,
			"live_path", livePath,
			"allowed_origin_count", len(allowedOrigins),
		)
		if err := server.ServeWebSocket(cmd.Context(), internalappserver.WebSocketServeOptions{
			ListenAddr:     target.ListenAddr,
			WebSocketPath:  websocketPath,
			ReadyPath:      readyPath,
			LivePath:       livePath,
			AllowedOrigins: allowedOrigins,
			MaxConnections: cfg.AppServer.Limits.MaxConnections,
			MaxFrameBytes:  cfg.AppServer.Limits.MaxFrameBytes,
		}); err != nil {
			return fmt.Errorf("failed to serve app-server; %w", err)
		}
		appServerServeLogger().Info("websocket app-server serve command completed", "listen_addr", target.ListenAddr)
		return nil
	default:
		return fmt.Errorf("failed to serve app-server; unsupported transport %q", target.Transport)
	}
}

func appServerServeLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.app_server.serve")
}
