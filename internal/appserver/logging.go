package appserver

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
)

func appServerLogger() *slog.Logger {
	return slog.Default().With("component", "appserver.server")
}

func appServerConnectionLogger(transport string, remoteAddr string) *slog.Logger {
	logger := appServerLogger().With("transport", transport)
	if strings.TrimSpace(remoteAddr) != "" {
		logger = logger.With("remote_addr", remoteAddr)
	}
	return logger
}

func appServerRemoteLogger() *slog.Logger {
	return slog.Default().With("component", "appserver.websocket")
}

type readLogContext struct {
	method      string
	querySource string
	runID       string
	nodeID      string
	stepID      string
	artifactRef string
	cursor      *string
	limit       *int
}

func (c readLogContext) attrs() []any {
	attrs := []any{
		"rpc_method", c.method,
		"query_source", c.querySource,
	}
	if runID := strings.TrimSpace(c.runID); runID != "" {
		attrs = append(attrs, "run_id", runID)
	}
	if nodeID := strings.TrimSpace(c.nodeID); nodeID != "" {
		attrs = append(attrs, "node_id", nodeID)
	}
	if stepID := strings.TrimSpace(c.stepID); stepID != "" {
		attrs = append(attrs, "step_id", stepID)
	}
	if artifactRef := strings.TrimSpace(c.artifactRef); artifactRef != "" {
		attrs = append(attrs, "artifact_ref", artifactRef)
	}
	if c.cursor != nil {
		if cursor := strings.TrimSpace(*c.cursor); cursor != "" {
			attrs = append(attrs, "cursor", cursor)
		}
	}
	if c.limit != nil {
		attrs = append(attrs, "limit", *c.limit)
	}
	return attrs
}

func logReadRequestSuccess(ctx readLogContext, extraAttrs ...any) {
	attrs := append(ctx.attrs(), extraAttrs...)
	appServerLogger().Info("completed app-server read request", attrs...)
}

func logReadRequestFailure(ctx readLogContext, err error, errObject *protocol.ErrorObject, extraAttrs ...any) {
	attrs := append(ctx.attrs(), errorLogAttrs(errObject)...)
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}
	attrs = append(attrs, extraAttrs...)
	appServerLogger().Warn("app-server read request failed", attrs...)
}

func requestLogAttrs(message inboundMessage, isNotification bool) []any {
	attrs := []any{
		"rpc_method", strings.TrimSpace(message.Method),
		"rpc_kind", requestKind(isNotification),
	}
	if version := strings.TrimSpace(message.JSONRPC); version != "" {
		attrs = append(attrs, "jsonrpc_version", version)
	}
	if len(message.ID) > 0 {
		attrs = append(attrs, "request_id", requestIDValue(message.ID))
	}
	return attrs
}

func requestKind(isNotification bool) string {
	if isNotification {
		return string(protocol.MethodKindNotification)
	}
	return string(protocol.MethodKindRequest)
}

func requestIDValue(id json.RawMessage) any {
	var decoded any
	if err := json.Unmarshal(id, &decoded); err == nil {
		return decoded
	}
	return string(id)
}

func errorLogAttrs(errObject *protocol.ErrorObject) []any {
	if errObject == nil {
		return nil
	}

	attrs := []any{"rpc_error_code", errObject.Code}
	if errObject.Data == nil {
		return attrs
	}
	if code := strings.TrimSpace(errObject.Data.Code); code != "" {
		attrs = append(attrs, "domain_code", code)
	}
	attrs = append(attrs, "retryable", errObject.Data.Retryable)
	if len(errObject.Data.Details) > 0 {
		attrs = append(attrs, "error_details", errObject.Data.Details)
	}
	return attrs
}
