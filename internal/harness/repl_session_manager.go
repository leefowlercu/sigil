package harness

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

var (
	// ErrInvalidManagerInput is returned when manager inputs are invalid.
	ErrInvalidManagerInput = errors.New("invalid manager input")
)

// NodeSessionInput contains per-node data required to initialize a REPL session.
type NodeSessionInput struct {
	RunID      string
	NodeID     string
	Depth      int
	Context    string
	RunContext context.Context
	Bindings   SubcallBindings
}

// SubcallBindings defines step-scoped subcall handlers used by node-local REPL sessions.
type SubcallBindings interface {
	LLMQuery(ctx context.Context, request repl.QueryRequest) (string, error)
	RLMQuery(ctx context.Context, request repl.QueryRequest) (string, error)
	LLMQueryBatched(ctx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error)
	RLMQueryBatched(ctx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error)
}

type sessionEntry struct {
	session    repl.Session
	dispatcher *subcallDispatcher
}

type subcallDispatcher struct {
	mu       sync.RWMutex
	bindings SubcallBindings
}

func (d *subcallDispatcher) Set(bindings SubcallBindings) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bindings = bindings
}

func (d *subcallDispatcher) current() (SubcallBindings, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.bindings == nil {
		return nil, fmt.Errorf("subcall bindings are not initialized; %w", ErrInvalidManagerInput)
	}
	return d.bindings, nil
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// REPLSessionManager owns one persistent REPL session per node id.
type REPLSessionManager struct {
	mu        sync.Mutex
	factory   repl.SessionFactory
	artifacts *ActionArtifactStore
	sessions  map[string]sessionEntry
}

// NewREPLSessionManager constructs a session manager.
func NewREPLSessionManager(factory repl.SessionFactory, artifacts *ActionArtifactStore) (*REPLSessionManager, error) {
	if factory == nil {
		return nil, fmt.Errorf("session factory is required; %w", ErrInvalidManagerInput)
	}
	if artifacts == nil {
		return nil, fmt.Errorf("action artifact store is required; %w", ErrInvalidManagerInput)
	}

	return &REPLSessionManager{
		factory:   factory,
		artifacts: artifacts,
		sessions:  make(map[string]sessionEntry),
	}, nil
}

// SessionForNode returns a persistent session for node id, creating one on first use.
func (m *REPLSessionManager) SessionForNode(ctx context.Context, input NodeSessionInput) (repl.Session, error) {
	if m == nil {
		return nil, fmt.Errorf("session manager is required; %w", ErrInvalidManagerInput)
	}
	if strings.TrimSpace(input.NodeID) == "" {
		return nil, fmt.Errorf("node id is required; %w", ErrInvalidManagerInput)
	}
	if input.Bindings == nil {
		return nil, fmt.Errorf("subcall bindings are required; %w", ErrInvalidManagerInput)
	}

	m.mu.Lock()
	entry, exists := m.sessions[input.NodeID]
	m.mu.Unlock()
	if exists {
		entry.dispatcher.Set(input.Bindings)
		replSessionManagerLogger().Debug("reusing existing node repl session",
			"run_id", input.RunID,
			"node_id", input.NodeID,
		)
		return entry.session, nil
	}

	dispatcher := &subcallDispatcher{}
	dispatcher.Set(input.Bindings)

	created, err := m.factory.NewSession(ctx, repl.SessionOptions{
		RunID:      input.RunID,
		NodeID:     input.NodeID,
		Depth:      input.Depth,
		Context:    input.Context,
		RunContext: input.RunContext,
		LLMQuery: func(callCtx context.Context, request repl.QueryRequest) (string, error) {
			current, currentErr := dispatcher.current()
			if currentErr != nil {
				return "", currentErr
			}
			return current.LLMQuery(callCtx, request)
		},
		RLMQuery: func(callCtx context.Context, request repl.QueryRequest) (string, error) {
			current, currentErr := dispatcher.current()
			if currentErr != nil {
				return "", currentErr
			}
			return current.RLMQuery(callCtx, request)
		},
		LLMQueryBatched: func(callCtx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
			current, currentErr := dispatcher.current()
			if currentErr != nil {
				return nil, currentErr
			}
			return current.LLMQueryBatched(callCtx, requests)
		},
		RLMQueryBatched: func(callCtx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
			current, currentErr := dispatcher.current()
			if currentErr != nil {
				return nil, currentErr
			}
			return current.RLMQueryBatched(callCtx, requests)
		},
		ReadActionArtifact: func(actionRef string) (repl.ActionOutput, error) {
			if strings.TrimSpace(actionRef) != actionRef {
				return repl.ActionOutput{}, fmt.Errorf("action_ref %q must be canonical without leading or trailing whitespace", actionRef)
			}
			if _, err := runtime.ParseActionArtifactRef(actionRef); err != nil {
				return repl.ActionOutput{}, err
			}

			artifact, err := m.artifacts.Read(input.RunID, actionRef)
			if err != nil {
				return repl.ActionOutput{}, err
			}

			return repl.ActionOutput{
				Status:       artifact.Status,
				Stdout:       artifact.Stdout,
				Stderr:       artifact.Stderr,
				ErrorCode:    dereferenceString(artifact.ErrorCode),
				ErrorMessage: dereferenceString(artifact.ErrorMessage),
			}, nil
		},
	})
	if err != nil {
		replSessionManagerLogger().Error("failed to create node repl session",
			"run_id", input.RunID,
			"node_id", input.NodeID,
			"node_depth", input.Depth,
			"error", err,
		)
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.sessions[input.NodeID]; ok {
		_ = created.Close()
		existing.dispatcher.Set(input.Bindings)
		return existing.session, nil
	}
	m.sessions[input.NodeID] = sessionEntry{
		session:    created,
		dispatcher: dispatcher,
	}
	replSessionManagerLogger().Info("created node repl session",
		"run_id", input.RunID,
		"node_id", input.NodeID,
		"node_depth", input.Depth,
		"active_sessions", len(m.sessions),
	)
	return created, nil
}

// CloseNode closes and forgets the session associated with node id.
func (m *REPLSessionManager) CloseNode(nodeID string) error {
	if m == nil {
		return fmt.Errorf("session manager is required; %w", ErrInvalidManagerInput)
	}
	if strings.TrimSpace(nodeID) == "" {
		return fmt.Errorf("node id is required; %w", ErrInvalidManagerInput)
	}

	m.mu.Lock()
	entry, exists := m.sessions[nodeID]
	if exists {
		delete(m.sessions, nodeID)
	}
	m.mu.Unlock()

	if !exists {
		return nil
	}
	if err := entry.session.Close(); err != nil {
		replSessionManagerLogger().Error("failed to close node repl session",
			"node_id", nodeID,
			"error", err,
		)
		return err
	}
	replSessionManagerLogger().Info("closed node repl session",
		"node_id", nodeID,
	)
	return nil
}

// CloseAll closes every managed session and resets manager state.
func (m *REPLSessionManager) CloseAll() error {
	if m == nil {
		return fmt.Errorf("session manager is required; %w", ErrInvalidManagerInput)
	}

	m.mu.Lock()
	sessions := make([]repl.Session, 0, len(m.sessions))
	for _, entry := range m.sessions {
		sessions = append(sessions, entry.session)
	}
	m.sessions = make(map[string]sessionEntry)
	m.mu.Unlock()

	var joinedErr error
	closedCount := 0
	for _, session := range sessions {
		if err := session.Close(); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			replSessionManagerLogger().Error("failed to close repl session during close all", "error", err)
			continue
		}
		closedCount++
	}
	replSessionManagerLogger().Info("closed all repl sessions", "closed_sessions", closedCount)

	return joinedErr
}

// SessionCount returns number of managed node sessions.
func (m *REPLSessionManager) SessionCount() int {
	if m == nil {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func replSessionManagerLogger() *slog.Logger {
	return slog.Default().With("component", "harness.repl_session_manager")
}
