package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/leefowlercu/sigil/internal/appserver/dispatcher"
	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/appserver/sessions"
	"github.com/leefowlercu/sigil/internal/buildinfo"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/harness"
)

type inboundMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type responseEnvelope struct {
	JSONRPC string                `json:"jsonrpc"`
	ID      json.RawMessage       `json:"id"`
	Result  interface{}           `json:"result,omitempty"`
	Error   *protocol.ErrorObject `json:"error,omitempty"`
}

type notificationEnvelope struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// ServerOptions configures optional app-server dependencies for tests and embedding.
type ServerOptions struct {
	Now           func() time.Time
	RunnerFactory func() *harness.Runner
}

// Server hosts the Sigil app-server core.
type Server struct {
	config        config.Config
	dispatcher    *dispatcher.Dispatcher
	now           func() time.Time
	runnerFactory func() *harness.Runner
	ownedRuns     *ownedRunManager
}

// New constructs a Server from active application configuration.
func New(cfg config.Config) *Server {
	return NewWithOptions(cfg, ServerOptions{})
}

// NewWithOptions constructs a Server from active application configuration and optional dependencies.
func NewWithOptions(cfg config.Config, options ServerOptions) *Server {
	dispatch := dispatcher.New()
	runnerFactory := options.RunnerFactory
	if runnerFactory == nil {
		runnerFactory = func() *harness.Runner {
			return harness.NewRunner(harness.WithRunsBaseDir(cfg.AppServer.RunDir))
		}
	}

	server := &Server{
		config:        cfg,
		dispatcher:    dispatch,
		runnerFactory: runnerFactory,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if options.Now != nil {
		server.now = options.Now
	}
	server.ownedRuns = newOwnedRunManager(server.runnerFactory)

	server.registerRunReadHandlers()
	server.registerRunLiveHandlers()
	dispatch.Register("server/ping", server.handleServerPing)
	return server
}

// ServeStdio serves the JSON-RPC protocol over stdio JSONL.
func (s *Server) ServeStdio(ctx context.Context, reader io.Reader, writer io.Writer) error {
	if s == nil {
		return fmt.Errorf("server is required")
	}
	if reader == nil {
		return fmt.Errorf("reader is required")
	}
	if writer == nil {
		return fmt.Errorf("writer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lineWriter := newStdioJSONRPCWriter(writer)

	connection := &connectionHandler{
		server:        s,
		logger:        appServerConnectionLogger("stdio", ""),
		connectionCtx: connectionCtx,
		writer:        lineWriter,
		onWriteFailure: func(error) {
			cancel()
		},
	}
	defer connection.closeSubscriptions()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, minInt(s.config.AppServer.Limits.MaxFrameBytes, 64*1024)), s.config.AppServer.Limits.MaxFrameBytes)
	for scanner.Scan() {
		select {
		case <-connectionCtx.Done():
			return nil
		default:
		}

		response, err := connection.handleLine(connectionCtx, append([]byte(nil), scanner.Bytes()...))
		if err != nil {
			return err
		}
		if len(response) == 0 {
			continue
		}
		if err := lineWriter.writeText(response); err != nil {
			return fmt.Errorf("failed to write response line; %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read request line; %w", err)
	}

	return nil
}

type connectionHandler struct {
	server         *Server
	logger         *slog.Logger
	connectionCtx  context.Context
	writer         outboundWriter
	onWriteFailure func(error)
	markOutbound   func()
	mu             sync.RWMutex
	state          sessions.State
	subscriptions  map[string]*connectionSubscription
}

func (c *connectionHandler) handleLine(ctx context.Context, line []byte) ([]byte, error) {
	var message inboundMessage
	if err := json.Unmarshal(line, &message); err != nil {
		c.logger.Warn("failed to decode app-server message",
			"payload_bytes", len(line),
			"domain_code", "parse_error",
			"error", err,
		)
		return marshalError(nil, protocol.NewError(protocol.CodeParseError, "parse error", "parse_error", false, nil))
	}
	isNotification := len(message.ID) == 0
	if message.JSONRPC != protocol.JSONRPCVersion {
		errObject := protocol.NewError(protocol.CodeInvalidRequest, "invalid request", "invalid_jsonrpc_version", false, nil)
		c.logRejectedRequest(message, isNotification, errObject)
		return marshalError(message.ID, errObject)
	}
	if strings.TrimSpace(message.Method) == "" {
		errObject := protocol.NewError(protocol.CodeInvalidRequest, "invalid request", "missing_method", false, nil)
		c.logRejectedRequest(message, isNotification, errObject)
		return marshalError(message.ID, errObject)
	}

	switch message.Method {
	case "initialize":
		if isNotification {
			c.logger.Warn("ignored initialize notification", requestLogAttrs(message, true)...)
			return nil, nil
		}
		return c.handleInitialize(message.ID, message.Params)
	case "initialized":
		c.mu.Lock()
		if !c.state.Initialized {
			c.mu.Unlock()
			c.logger.Warn("ignored initialized notification before initialize", requestLogAttrs(message, true)...)
			return nil, nil
		}
		if c.state.Ready {
			protocolVersion := c.state.ProtocolVersion
			clientName := c.state.Client.Name
			clientVersion := c.state.Client.Version
			c.mu.Unlock()
			c.logger.Debug("ignored repeated initialized notification",
				"protocol_version", protocolVersion,
				"client_name", clientName,
				"client_version", clientVersion,
			)
			return nil, nil
		}
		c.state.Ready = true
		protocolVersion := c.state.ProtocolVersion
		clientName := c.state.Client.Name
		clientVersion := c.state.Client.Version
		c.mu.Unlock()
		c.logger.Info("app-server connection ready",
			"protocol_version", protocolVersion,
			"client_name", clientName,
			"client_version", clientVersion,
		)
		return nil, nil
	}

	c.mu.RLock()
	initialized := c.state.Initialized
	ready := c.state.Ready
	c.mu.RUnlock()

	if !initialized {
		errObject := protocol.NewError(protocol.CodeInvalidRequest, "initialize is required before other requests", "handshake_required", false, nil)
		c.logRejectedRequest(message, isNotification, errObject)
		return marshalError(message.ID, errObject)
	}
	if !ready {
		errObject := protocol.NewError(protocol.CodeInvalidRequest, "initialized notification is required before other requests", "initialized_required", false, nil)
		c.logRejectedRequest(message, isNotification, errObject)
		return marshalError(message.ID, errObject)
	}
	if isNotification {
		c.logger.Debug("ignored app-server notification without handler", requestLogAttrs(message, true)...)
		return nil, nil
	}

	dispatchCtx := withConnectionHandler(ctx, c)
	result, dispatchErr := c.server.dispatcher.Dispatch(dispatchCtx, message.Method, message.Params)
	if dispatchErr != nil {
		c.logRequestFailure(message, dispatchErr)
		return marshalError(message.ID, dispatchErr)
	}
	c.logger.Debug("app-server request completed", requestLogAttrs(message, false)...)
	return marshalResult(message.ID, result)
}

func (c *connectionHandler) handleInitialize(id json.RawMessage, rawParams json.RawMessage) ([]byte, error) {
	c.mu.Lock()
	if c.state.Initialized {
		c.mu.Unlock()
		errObject := protocol.NewError(protocol.CodeInvalidRequest, "initialize may only be called once per connection", "already_initialized", false, nil)
		c.logger.Warn("failed to initialize app-server connection", errorLogAttrs(errObject)...)
		return marshalError(id, errObject)
	}

	params, errObject := decodeParams[protocol.InitializeParams](rawParams)
	if errObject != nil {
		c.mu.Unlock()
		c.logger.Warn("failed to initialize app-server connection", errorLogAttrs(errObject)...)
		return marshalError(id, errObject)
	}
	version, ok := negotiateProtocolVersion(params.ProtocolVersions)
	if !ok {
		c.mu.Unlock()
		errObject := protocol.NewError(protocol.CodeInvalidParams, "no compatible protocol version was supplied", "unsupported_protocol_version", false, map[string]interface{}{
			"supportedVersions": protocol.SupportedProtocolVersions(),
		})
		c.logger.Warn("failed to initialize app-server connection", errorLogAttrs(errObject)...)
		return marshalError(id, errObject)
	}

	c.state.Initialized = true
	c.state.ProtocolVersion = version
	c.state.Client = params.Client
	c.mu.Unlock()
	c.logger.Info("app-server connection initialized",
		"protocol_version", version,
		"client_name", params.Client.Name,
		"client_version", params.Client.Version,
	)

	result := protocol.InitializeResult{
		ServerName:      "sigil",
		ServerVersion:   buildinfo.Version,
		InstanceID:      c.server.config.AppServer.InstanceID,
		InstanceName:    c.server.config.AppServer.InstanceName,
		ProtocolVersion: version,
		MethodFamilies:  []string{"run", "server"},
		Capabilities: protocol.ServerCapabilities{
			Config: protocol.ConfigCapability{
				DefaultVersion:    protocol.DefaultProtocolVersion(),
				SupportedVersions: protocol.SupportedProtocolVersions(),
			},
			Views: protocol.ViewCapabilities{
				RunTree:           true,
				StepDetail:        true,
				LiveSubscriptions: true,
			},
			ProtocolArtifacts: true,
		},
	}
	return marshalResult(id, result)
}

func (c *connectionHandler) logRejectedRequest(message inboundMessage, isNotification bool, errObject *protocol.ErrorObject) {
	attrs := append(requestLogAttrs(message, isNotification), errorLogAttrs(errObject)...)
	c.logger.Warn("rejected app-server request", attrs...)
}

func (c *connectionHandler) logRequestFailure(message inboundMessage, errObject *protocol.ErrorObject) {
	attrs := append(requestLogAttrs(message, false), errorLogAttrs(errObject)...)
	c.logger.Warn("app-server request failed", attrs...)
}

func (c *connectionHandler) isReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.state.Ready
}

func (s *Server) handleServerPing(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	if _, errObject := decodeParams[protocol.ServerPingParams](rawParams); errObject != nil {
		return nil, errObject
	}

	return protocol.ServerPingResult{
		ServerName:      "sigil",
		ServerVersion:   buildinfo.Version,
		InstanceID:      s.config.AppServer.InstanceID,
		ProtocolVersion: protocol.DefaultProtocolVersion(),
		ServerTime:      s.now(),
	}, nil
}

func (s *Server) heartbeatParams(interval time.Duration) protocol.ServerHeartbeatParams {
	return protocol.ServerHeartbeatParams{
		InstanceID:          s.config.AppServer.InstanceID,
		ServerTime:          s.now(),
		HeartbeatIntervalMS: int(interval / time.Millisecond),
	}
}

func decodeParams[T any](rawParams json.RawMessage) (T, *protocol.ErrorObject) {
	var zero T
	trimmed := strings.TrimSpace(string(rawParams))
	if trimmed == "" || trimmed == "null" {
		return zero, nil
	}
	if err := json.Unmarshal(rawParams, &zero); err != nil {
		return zero, protocol.NewError(protocol.CodeInvalidParams, "invalid params", "invalid_params", false, map[string]interface{}{
			"error": err.Error(),
		})
	}
	return zero, nil
}

func negotiateProtocolVersion(clientVersions []string) (string, bool) {
	supported := protocol.SupportedProtocolVersions()
	for _, clientVersion := range clientVersions {
		for _, supportedVersion := range supported {
			if strings.TrimSpace(clientVersion) == supportedVersion {
				return supportedVersion, true
			}
		}
	}
	return "", false
}

func marshalResult(id json.RawMessage, result interface{}) ([]byte, error) {
	response := responseEnvelope{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      cloneJSON(id),
		Result:  result,
	}
	return marshalResponse(response)
}

func marshalError(id json.RawMessage, errObject *protocol.ErrorObject) ([]byte, error) {
	response := responseEnvelope{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      normalizeID(id),
		Error:   errObject,
	}
	return marshalResponse(response)
}

func marshalResponse(response responseEnvelope) ([]byte, error) {
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON-RPC response; %w", err)
	}
	return encoded, nil
}

func marshalNotification(method string, params interface{}) ([]byte, error) {
	encoded, err := json.Marshal(notificationEnvelope{
		JSONRPC: protocol.JSONRPCVersion,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON-RPC notification; %w", err)
	}
	return encoded, nil
}

func normalizeID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return cloneJSON(id)
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	cloned := make(json.RawMessage, len(value))
	copy(cloned, value)
	return cloned
}
