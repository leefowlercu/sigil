package appserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultHeartbeatInterval = 2 * time.Second
	websocketWriteTimeout    = 5 * time.Second
)

// WebSocketServeOptions defines remote listener settings for the app-server.
type WebSocketServeOptions struct {
	Listener          net.Listener
	ListenAddr        string
	WebSocketPath     string
	ReadyPath         string
	LivePath          string
	AllowedOrigins    []string
	MaxConnections    int
	MaxFrameBytes     int
	HeartbeatInterval time.Duration
	OnListen          func(net.Addr)
}

// ServeWebSocket serves the JSON-RPC protocol over a localhost/dev WebSocket listener.
func (s *Server) ServeWebSocket(ctx context.Context, options WebSocketServeOptions) error {
	if s == nil {
		return fmt.Errorf("server is required")
	}
	if err := validateWebSocketServeOptions(options); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureRunSummaryIndex(ctx); err != nil {
		return err
	}

	listener := options.Listener
	if listener == nil {
		created, err := net.Listen("tcp", options.ListenAddr)
		if err != nil {
			return fmt.Errorf("failed to listen for app-server websocket requests; %w", err)
		}
		listener = created
	}
	if options.OnListen != nil {
		options.OnListen(listener.Addr())
	}

	allowedOrigins, err := normalizeAllowedOrigins(options.AllowedOrigins)
	if err != nil {
		_ = listener.Close()
		return err
	}

	var activeConnections atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc(options.ReadyPath, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
	})
	mux.HandleFunc(options.LivePath, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
	})

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	mux.HandleFunc(options.WebSocketPath, func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if !originAllowed(origin, allowedOrigins) {
			appServerRemoteLogger().Warn("rejected websocket app-server connection",
				"remote_addr", request.RemoteAddr,
				"origin", origin,
				"reason", "disallowed_origin",
			)
			http.Error(writer, "app-server websocket origin is not allowed", http.StatusForbidden)
			return
		}

		currentConnections := activeConnections.Add(1)
		if currentConnections > int32(options.MaxConnections) {
			activeConnections.Add(-1)
			appServerRemoteLogger().Warn("rejected websocket app-server connection",
				"remote_addr", request.RemoteAddr,
				"origin", origin,
				"reason", "connection_limit_reached",
				"active_connections", currentConnections-1,
				"max_connections", options.MaxConnections,
			)
			http.Error(writer, "app-server connection limit reached", http.StatusServiceUnavailable)
			return
		}

		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			activeConnections.Add(-1)
			appServerRemoteLogger().Warn("failed to upgrade websocket request", "error", err)
			return
		}

		transportLogger := appServerRemoteLogger().With("remote_addr", request.RemoteAddr, "origin", origin)
		rpcLogger := appServerConnectionLogger("websocket", request.RemoteAddr).With("origin", origin)
		transportLogger.Info("accepted websocket app-server connection", "active_connections", currentConnections)

		go func() {
			defer connection.Close()
			defer func() {
				remainingConnections := activeConnections.Add(-1)
				transportLogger.Info("websocket app-server connection closed", "active_connections", remainingConnections)
			}()

			if err := s.serveWebSocketConnection(ctx, connection, options.HeartbeatInterval, options.MaxFrameBytes, rpcLogger); err != nil {
				transportLogger.Warn("websocket connection ended with error", "error", err)
			}
		}()
	})

	server := &http.Server{
		Handler: mux,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	appServerRemoteLogger().Info("serving websocket app-server listener",
		"listen_addr", listener.Addr().String(),
		"websocket_path", options.WebSocketPath,
		"ready_path", options.ReadyPath,
		"live_path", options.LivePath,
	)

	err = server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to serve websocket app-server listener; %w", err)
	}

	<-shutdownDone
	appServerRemoteLogger().Info("stopped websocket app-server listener", "listen_addr", listener.Addr().String())
	return nil
}

func (s *Server) serveWebSocketConnection(
	ctx context.Context,
	connection *websocket.Conn,
	heartbeatInterval time.Duration,
	maxFrameBytes int,
	logger *slog.Logger,
) error {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}

	connection.SetReadLimit(int64(maxFrameBytes))
	writer := &websocketJSONRPCWriter{connection: connection}
	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lastOutbound := &atomic.Int64{}
	lastOutbound.Store(time.Now().UTC().UnixNano())
	handler := &connectionHandler{
		server:        s,
		logger:        logger,
		connectionCtx: connectionCtx,
		writer:        writer,
		onWriteFailure: func(error) {
			cancel()
		},
		markOutbound: func() {
			lastOutbound.Store(time.Now().UTC().UnixNano())
		},
	}
	defer handler.closeSubscriptions()

	go s.runHeartbeatLoop(connectionCtx, writer, handler, heartbeatInterval, lastOutbound, cancel, logger)

	for {
		messageType, payload, err := connection.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return err
		}
		if messageType != websocket.TextMessage {
			logger.Warn("rejected websocket frame",
				"frame_type", messageType,
				"reason", "unsupported_message_type",
			)
			_ = writer.writeClose(websocket.CloseUnsupportedData, "only websocket text frames are supported")
			return nil
		}

		response, err := handler.handleLine(connectionCtx, payload)
		if err != nil {
			return err
		}
		if len(response) == 0 {
			continue
		}
		if err := writer.writeText(response); err != nil {
			return err
		}
		lastOutbound.Store(time.Now().UTC().UnixNano())
	}
}

func (s *Server) runHeartbeatLoop(
	ctx context.Context,
	writer outboundWriter,
	handler *connectionHandler,
	heartbeatInterval time.Duration,
	lastOutbound *atomic.Int64,
	cancel context.CancelFunc,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !handler.isReady() {
				continue
			}
			lastSentAt := time.Unix(0, lastOutbound.Load())
			if time.Since(lastSentAt) < heartbeatInterval {
				continue
			}

			heartbeat, err := marshalNotification("server/heartbeat", s.heartbeatParams(heartbeatInterval))
			if err != nil {
				logger.Warn("failed to marshal websocket heartbeat", "error", err)
				cancel()
				return
			}
			if err := writer.writeText(heartbeat); err != nil {
				logger.Warn("failed to send websocket heartbeat", "error", err)
				cancel()
				return
			}
			logger.Debug("sent websocket heartbeat", "heartbeat_interval_ms", int(heartbeatInterval/time.Millisecond))
			lastOutbound.Store(time.Now().UTC().UnixNano())
		}
	}
}

func validateWebSocketServeOptions(options WebSocketServeOptions) error {
	if options.Listener == nil {
		if strings.TrimSpace(options.ListenAddr) == "" {
			return fmt.Errorf("listen address is required")
		}
		if err := validateLoopbackListenAddr(options.ListenAddr); err != nil {
			return err
		}
	}
	if strings.TrimSpace(options.WebSocketPath) == "" || !strings.HasPrefix(strings.TrimSpace(options.WebSocketPath), "/") {
		return fmt.Errorf("websocket path must start with /")
	}
	if strings.TrimSpace(options.ReadyPath) == "" || !strings.HasPrefix(strings.TrimSpace(options.ReadyPath), "/") {
		return fmt.Errorf("ready path must start with /")
	}
	if strings.TrimSpace(options.LivePath) == "" || !strings.HasPrefix(strings.TrimSpace(options.LivePath), "/") {
		return fmt.Errorf("live path must start with /")
	}
	if options.MaxConnections < 1 {
		return fmt.Errorf("max connections must be >= 1")
	}
	if options.MaxFrameBytes < 1 {
		return fmt.Errorf("max frame bytes must be >= 1")
	}
	return nil
}

func validateLoopbackListenAddr(listenAddr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return fmt.Errorf("invalid websocket listen address %q; %w", listenAddr, err)
	}
	if host == "" {
		return fmt.Errorf("websocket listen address %q must include an explicit host", listenAddr)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	parsedIP := net.ParseIP(host)
	if parsedIP != nil && parsedIP.IsLoopback() {
		return nil
	}
	return fmt.Errorf("websocket listen address %q must target localhost or a loopback IP", listenAddr)
}

func normalizeAllowedOrigins(origins []string) (map[string]struct{}, error) {
	normalized := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		value, err := normalizeOrigin(origin)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		normalized[value] = struct{}{}
	}
	return normalized, nil
}

func normalizeOrigin(origin string) (string, error) {
	trimmed := strings.TrimSpace(origin)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid allowed origin %q; %w", trimmed, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("allowed origin %q must include scheme and host", trimmed)
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func originAllowed(origin string, allowedOrigins map[string]struct{}) bool {
	normalized, err := normalizeOrigin(origin)
	if err != nil {
		return false
	}
	if normalized == "" {
		return true
	}
	if len(allowedOrigins) == 0 {
		return false
	}
	_, ok := allowedOrigins[normalized]
	return ok
}

type websocketJSONRPCWriter struct {
	connection *websocket.Conn
	mu         sync.Mutex
}

func (w *websocketJSONRPCWriter) writeText(message []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.connection.SetWriteDeadline(time.Now().Add(websocketWriteTimeout)); err != nil {
		return err
	}
	return w.connection.WriteMessage(websocket.TextMessage, message)
}

func (w *websocketJSONRPCWriter) writeClose(code int, text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	deadline := time.Now().Add(websocketWriteTimeout)
	return w.connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), deadline)
}
