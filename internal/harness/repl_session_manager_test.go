package harness

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/leefowlercu/sigil/internal/repl"
)

type fakeSessionFactory struct {
	newCalls int
	sessions []*fakeSession
}

func (f *fakeSessionFactory) NewSession(_ context.Context, _ repl.SessionOptions) (repl.Session, error) {
	f.newCalls++
	session := &fakeSession{}
	f.sessions = append(f.sessions, session)
	return session, nil
}

type fakeSession struct {
	closed bool
	closeN int
	closeE error
}

func (s *fakeSession) Exec(_ context.Context, _ string) (repl.ExecResult, error) {
	if s.closed {
		return repl.ExecResult{}, repl.ErrSessionClosed
	}
	return repl.ExecResult{}, nil
}

func (s *fakeSession) Close() error {
	s.closeN++
	s.closed = true
	return s.closeE
}

type managerNoopSubcalls struct{}

func (managerNoopSubcalls) LLMQuery(_ context.Context, _ repl.QueryRequest) (string, error) {
	return "answer", nil
}

func (managerNoopSubcalls) RLMQuery(_ context.Context, _ repl.QueryRequest) (string, error) {
	return "answer", nil
}

func (managerNoopSubcalls) LLMQueryBatched(_ context.Context, _ []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	return nil, nil
}

func (managerNoopSubcalls) RLMQueryBatched(_ context.Context, _ []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	return nil, nil
}

func TestNewREPLSessionManagerRequiresFactory(t *testing.T) {
	_, err := NewREPLSessionManager(nil)
	if !errors.Is(err, ErrInvalidManagerInput) {
		t.Fatalf("expected ErrInvalidManagerInput, got %v", err)
	}
}

func TestSessionForNodeCreatesAndReusesSessionPerNodeID(t *testing.T) {
	factory := &fakeSessionFactory{}
	manager, err := NewREPLSessionManager(factory)
	if err != nil {
		t.Fatalf("expected manager construction success, got %v", err)
	}

	input := NodeSessionInput{
		RunID:    "run-1",
		NodeID:   "node-1",
		Depth:    0,
		Context:  "context",
		Bindings: managerNoopSubcalls{},
	}

	first, err := manager.SessionForNode(context.Background(), input)
	if err != nil {
		t.Fatalf("expected first session creation success, got %v", err)
	}
	second, err := manager.SessionForNode(context.Background(), input)
	if err != nil {
		t.Fatalf("expected second session lookup success, got %v", err)
	}

	if first != second {
		t.Fatalf("expected same session instance for repeated node id lookup")
	}
	if factory.newCalls != 1 {
		t.Fatalf("expected one factory call, got %d", factory.newCalls)
	}
	if manager.SessionCount() != 1 {
		t.Fatalf("expected one managed session, got %d", manager.SessionCount())
	}
}

func TestCloseNodeClosesAndRemovesSession(t *testing.T) {
	factory := &fakeSessionFactory{}
	manager, err := NewREPLSessionManager(factory)
	if err != nil {
		t.Fatalf("expected manager construction success, got %v", err)
	}

	_, err = manager.SessionForNode(context.Background(), NodeSessionInput{
		RunID:    "run-1",
		NodeID:   "node-1",
		Depth:    0,
		Context:  "context",
		Bindings: managerNoopSubcalls{},
	})
	if err != nil {
		t.Fatalf("expected session creation success, got %v", err)
	}

	if err := manager.CloseNode("node-1"); err != nil {
		t.Fatalf("expected close node success, got %v", err)
	}
	if manager.SessionCount() != 0 {
		t.Fatalf("expected zero sessions after close, got %d", manager.SessionCount())
	}
	if factory.sessions[0].closeN != 1 {
		t.Fatalf("expected one close call, got %d", factory.sessions[0].closeN)
	}
}

func TestCloseAllClosesManagedSessionsAndReturnsJoinedErrors(t *testing.T) {
	factory := &fakeSessionFactory{}
	manager, err := NewREPLSessionManager(factory)
	if err != nil {
		t.Fatalf("expected manager construction success, got %v", err)
	}

	for index := 0; index < 2; index++ {
		_, err = manager.SessionForNode(context.Background(), NodeSessionInput{
			RunID:    "run-1",
			NodeID:   fmt.Sprintf("node-%d", index),
			Depth:    0,
			Context:  "context",
			Bindings: managerNoopSubcalls{},
		})
		if err != nil {
			t.Fatalf("expected session creation success, got %v", err)
		}
	}

	targetErr := errors.New("close failed")
	factory.sessions[1].closeE = targetErr
	closeErr := manager.CloseAll()
	if !errors.Is(closeErr, targetErr) {
		t.Fatalf("expected joined error containing close failure, got %v", closeErr)
	}
	if manager.SessionCount() != 0 {
		t.Fatalf("expected zero sessions after close all, got %d", manager.SessionCount())
	}
}
