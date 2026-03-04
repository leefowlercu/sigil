package repl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateSessionOptions(t *testing.T) {
	query := func(_ context.Context, _ QueryRequest) (string, error) {
		return "ok", nil
	}
	batched := func(_ context.Context, _ []BatchedQueryRequest) ([]BatchedQueryResult, error) {
		return nil, nil
	}

	testCases := []struct {
		name    string
		options SessionOptions
		wantErr bool
	}{
		{
			name: "valid options",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				Depth:           0,
				Context:         "ctx",
				LLMQuery:        query,
				RLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: false,
		},
		{
			name: "missing run id",
			options: SessionOptions{
				NodeID:          "node-1",
				LLMQuery:        query,
				RLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "missing node id",
			options: SessionOptions{
				RunID:           "run-1",
				LLMQuery:        query,
				RLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "negative depth",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				Depth:           -1,
				LLMQuery:        query,
				RLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "missing llm query",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				RLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "missing rlm query",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				LLMQuery:        query,
				LLMQueryBatched: batched,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "missing llm_query_batched",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				LLMQuery:        query,
				RLMQuery:        query,
				RLMQueryBatched: batched,
			},
			wantErr: true,
		},
		{
			name: "missing rlm_query_batched",
			options: SessionOptions{
				RunID:           "run-1",
				NodeID:          "node-1",
				LLMQuery:        query,
				RLMQuery:        query,
				LLMQueryBatched: batched,
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSessionOptions(tc.options)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidSessionOptions) {
					t.Fatalf("expected ErrInvalidSessionOptions, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestYaegiSessionExecPersistsStateAcrossSteps(t *testing.T) {
	session := mustNewSession(t)

	result, err := session.Exec(context.Background(), `import "fmt"; counter := 1; fmt.Print(counter)`)
	if err != nil {
		t.Fatalf("expected first exec success, got %v", err)
	}
	if result.Stdout != "1" {
		t.Fatalf("expected stdout 1, got %q", result.Stdout)
	}

	result, err = session.Exec(context.Background(), `counter = counter + 1; fmt.Print(counter)`)
	if err != nil {
		t.Fatalf("expected second exec success, got %v", err)
	}
	if result.Stdout != "2" {
		t.Fatalf("expected stdout 2, got %q", result.Stdout)
	}
}

func TestYaegiSessionExecAllowsRepeatedImportsAcrossSteps(t *testing.T) {
	session := mustNewSession(t)

	result, err := session.Exec(context.Background(), `import "fmt"; fmt.Print("first")`)
	if err != nil {
		t.Fatalf("expected first exec success, got %v", err)
	}
	if result.Stdout != "first" {
		t.Fatalf("expected stdout first, got %q", result.Stdout)
	}

	result, err = session.Exec(context.Background(), `import "fmt"; fmt.Print("second")`)
	if err != nil {
		t.Fatalf("expected second exec success with repeated import, got %v", err)
	}
	if result.Stdout != "second" {
		t.Fatalf("expected stdout second, got %q", result.Stdout)
	}
}

func TestYaegiSessionExposesContextBinding(t *testing.T) {
	session := mustNewSessionWithContext(t, "ctx-value")

	result, err := session.Exec(context.Background(), `import "fmt"; fmt.Print(context)`)
	if err != nil {
		t.Fatalf("expected context print success, got %v", err)
	}
	if result.Stdout != "ctx-value" {
		t.Fatalf("expected context output ctx-value, got %q", result.Stdout)
	}
}

func TestYaegiSessionExposesRLMQueryBinding(t *testing.T) {
	session := mustNewSessionWithQuery(t, func(_ context.Context, request QueryRequest) (string, error) {
		return request.Prompt + "|" + request.Context, nil
	})

	result, err := session.Exec(context.Background(), `import "fmt"; answer, err := rlm_query("prompt", "subctx"); if err != nil { panic(err) }; fmt.Print(answer)`)
	if err != nil {
		t.Fatalf("expected rlm_query exec success, got %v", err)
	}
	if result.Stdout != "prompt|subctx" {
		t.Fatalf("expected rlm_query output prompt|subctx, got %q", result.Stdout)
	}
}

func TestYaegiSessionExecSupportsRLMQueryAssignmentInsideLoop(t *testing.T) {
	session := mustNewSessionWithQuery(t, func(_ context.Context, request QueryRequest) (string, error) {
		return request.Prompt + "|" + request.Context, nil
	})

	code := `
import (
	"fmt"
	"strings"
)

for i := 0; i < 1; i++ {
	prompt := "p"
	var ans string
	var queryErr error
	ans, queryErr = rlm_query(prompt, context)
	if queryErr != nil {
		fmt.Println("err", queryErr)
		continue
	}
	a := strings.TrimSpace(ans)
	fmt.Println(a)
}
`
	result, err := session.Exec(context.Background(), code)
	if err != nil {
		t.Fatalf("expected looped rlm_query exec success, got %v", err)
	}
	if !strings.Contains(result.Stdout, "p|context") {
		t.Fatalf("expected stdout to contain looped answer, got %q", result.Stdout)
	}
}

func TestYaegiSessionExecRejectsRLMQueryTupleDeclarationInsideLoop(t *testing.T) {
	session := mustNewSessionWithQuery(t, func(_ context.Context, request QueryRequest) (string, error) {
		return request.Prompt + "|" + request.Context, nil
	})

	code := `
import (
	"fmt"
	"strings"
)

for i := 0; i < 1; i++ {
	prompt := "p"
	ans, err := rlm_query(prompt, context)
	if err != nil {
		fmt.Println("err", err)
	} else {
		a := strings.TrimSpace(ans)
		fmt.Println(a)
	}
}
`
	_, err := session.Exec(context.Background(), code)
	if !IsCode(err, ErrorCodeExecutionCompile) {
		t.Fatalf("expected compile error for tuple declaration inside loop, got %v", err)
	}
	if !strings.Contains(err.Error(), "undefined: err") {
		t.Fatalf("expected undefined err compile detail, got %v", err)
	}
}

func TestYaegiSessionExposesLLMQueryBinding(t *testing.T) {
	session := mustNewSessionWithLLMQuery(t, func(_ context.Context, request QueryRequest) (string, error) {
		return request.Prompt + "|" + request.Context, nil
	})

	result, err := session.Exec(context.Background(), `import "fmt"; answer, err := llm_query("prompt", "subctx"); if err != nil { panic(err) }; fmt.Print(answer)`)
	if err != nil {
		t.Fatalf("expected llm_query exec success, got %v", err)
	}
	if result.Stdout != "prompt|subctx" {
		t.Fatalf("expected llm_query output prompt|subctx, got %q", result.Stdout)
	}
}

func TestYaegiSessionExposesBatchedBindings(t *testing.T) {
	session := mustNewSessionWithBatchedQueries(t,
		func(_ context.Context, requests []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			output := make([]BatchedQueryResult, len(requests))
			for index, request := range requests {
				output[index] = BatchedQueryResult{Answer: "llm:" + request.Prompt + "|" + request.Context}
			}
			return output, nil
		},
		func(_ context.Context, requests []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			output := make([]BatchedQueryResult, len(requests))
			for index, request := range requests {
				output[index] = BatchedQueryResult{Answer: "rlm:" + request.Prompt + "|" + request.Context}
			}
			return output, nil
		},
	)

	result, err := session.Exec(context.Background(), `
import "fmt"
calls := []map[string]string{
	{"prompt":"p1","context":"c1"},
	{"prompt":"p2","context":"c2"},
}
llm, err := llm_query_batched(calls)
if err != nil { panic(err) }
rlm, err := rlm_query_batched(calls)
if err != nil { panic(err) }
fmt.Print(llm[0]["answer"] + "," + llm[1]["answer"] + ";" + rlm[0]["answer"] + "," + rlm[1]["answer"])
`)
	if err != nil {
		t.Fatalf("expected batched binding exec success, got %v", err)
	}
	expected := "llm:p1|c1,llm:p2|c2;rlm:p1|c1,rlm:p2|c2"
	if result.Stdout != expected {
		t.Fatalf("expected batched output %q, got %q", expected, result.Stdout)
	}
}

func TestYaegiSessionRejectsBlockedImports(t *testing.T) {
	session := mustNewSession(t)

	_, err := session.Exec(context.Background(), `import "os"`)
	if !IsCode(err, ErrorCodeImportBlocked) {
		t.Fatalf("expected import-blocked error, got %v", err)
	}
}

func TestYaegiSessionRejectsBlockedAliasedImports(t *testing.T) {
	session := mustNewSession(t)

	_, err := session.Exec(context.Background(), `import o "os"; var _ = o.PathSeparator`)
	if !IsCode(err, ErrorCodeImportBlocked) {
		t.Fatalf("expected import-blocked error for aliased import, got %v", err)
	}
}

func TestYaegiSessionRejectsOversizedCode(t *testing.T) {
	factory := NewFactory(WithMaxCodeBytes(8))
	session := mustNewSessionWithFactory(t, factory)

	_, err := session.Exec(context.Background(), `import "fmt"`)
	if !IsCode(err, ErrorCodeCodeSizeExceeded) {
		t.Fatalf("expected code-size error, got %v", err)
	}
}

func TestYaegiSessionTruncatesOutputAtConfiguredCap(t *testing.T) {
	factory := NewFactory(WithOutputCaps(4, 4))
	session := mustNewSessionWithFactory(t, factory)

	result, err := session.Exec(context.Background(), `import "fmt"; fmt.Print("123456")`)
	if err != nil {
		t.Fatalf("expected exec success, got %v", err)
	}
	if !strings.HasPrefix(result.Stdout, "1234") {
		t.Fatalf("expected stdout prefix 1234, got %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, OutputTruncationMarker) {
		t.Fatalf("expected truncation marker in stdout, got %q", result.Stdout)
	}
}

func TestYaegiSessionReturnsSessionClosedAfterClose(t *testing.T) {
	session := mustNewSession(t)
	if err := session.Close(); err != nil {
		t.Fatalf("expected close success, got %v", err)
	}

	_, err := session.Exec(context.Background(), `1+1`)
	if !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed, got %v", err)
	}
}

func TestYaegiSessionTimesOutUsingConfiguredActionTimeout(t *testing.T) {
	factory := NewFactory(WithActionTimeout(20 * time.Millisecond))
	session := mustNewSessionWithFactoryAndQuery(t, factory, func(_ context.Context, _ QueryRequest) (string, error) {
		return "", nil
	})

	_, err := session.Exec(context.Background(), `for {}`)
	if !IsCode(err, ErrorCodeExecutionTimeout) {
		t.Fatalf("expected execution-timeout error, got %v", err)
	}
}

func TestParseBatchedCalls(t *testing.T) {
	requests, err := ParseBatchedCalls([]map[string]string{
		{"prompt": "p1", "context": "c1"},
		{"prompt": "p2", "context": "c2"},
	})
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].Prompt != "p1" || requests[0].Context != "c1" {
		t.Fatalf("unexpected request[0] %+v", requests[0])
	}
}

func TestParseBatchedCallsRejectsInvalidInput(t *testing.T) {
	_, err := ParseBatchedCalls([]map[string]string{{"prompt": "p1"}})
	if err == nil {
		t.Fatal("expected invalid input error")
	}
}

func mustNewSession(t *testing.T) Session {
	t.Helper()
	return mustNewSessionWithFactoryAndQuery(t, NewFactory(), func(_ context.Context, _ QueryRequest) (string, error) {
		return "ok", nil
	})
}

func mustNewSessionWithContext(t *testing.T, contextValue string) Session {
	t.Helper()
	factory := NewFactory()
	query := func(_ context.Context, _ QueryRequest) (string, error) {
		return "ok", nil
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	session, err := factory.NewSession(context.Background(), SessionOptions{
		RunID:    runID,
		NodeID:   nodeID,
		Depth:    0,
		Context:  contextValue,
		LLMQuery: query,
		RLMQuery: query,
		LLMQueryBatched: func(_ context.Context, _ []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			return nil, nil
		},
		RLMQueryBatched: func(_ context.Context, _ []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("expected session creation success, got %v", err)
	}
	return session
}

func mustNewSessionWithQuery(t *testing.T, query QueryFunc) Session {
	t.Helper()
	return mustNewSessionWithFactoryAndQuery(t, NewFactory(), query)
}

func mustNewSessionWithLLMQuery(t *testing.T, query QueryFunc) Session {
	t.Helper()
	return mustNewSessionWithFactoryAndQueries(t, NewFactory(), query, func(_ context.Context, _ QueryRequest) (string, error) {
		return "ok", nil
	})
}

func mustNewSessionWithFactory(t *testing.T, factory *Factory) Session {
	t.Helper()
	return mustNewSessionWithFactoryAndQuery(t, factory, func(_ context.Context, _ QueryRequest) (string, error) {
		return "ok", nil
	})
}

func mustNewSessionWithFactoryAndQuery(t *testing.T, factory *Factory, query QueryFunc) Session {
	t.Helper()
	return mustNewSessionWithFactoryAndQueries(t, factory, query, query)
}

func mustNewSessionWithFactoryAndQueries(t *testing.T, factory *Factory, llmQuery QueryFunc, rlmQuery QueryFunc) Session {
	t.Helper()

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	session, err := factory.NewSession(context.Background(), SessionOptions{
		RunID:    runID,
		NodeID:   nodeID,
		Depth:    0,
		Context:  "context",
		LLMQuery: llmQuery,
		RLMQuery: rlmQuery,
		LLMQueryBatched: func(_ context.Context, requests []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			results := make([]BatchedQueryResult, len(requests))
			for index := range requests {
				results[index] = BatchedQueryResult{}
			}
			return results, nil
		},
		RLMQueryBatched: func(_ context.Context, requests []BatchedQueryRequest) ([]BatchedQueryResult, error) {
			results := make([]BatchedQueryResult, len(requests))
			for index := range requests {
				results[index] = BatchedQueryResult{}
			}
			return results, nil
		},
	})
	if err != nil {
		t.Fatalf("expected session creation success, got %v", err)
	}

	return session
}

func mustNewSessionWithBatchedQueries(
	t *testing.T,
	llmBatch BatchedQueryFunc,
	rlmBatch BatchedQueryFunc,
) Session {
	t.Helper()
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	session, err := NewFactory().NewSession(context.Background(), SessionOptions{
		RunID:           runID,
		NodeID:          nodeID,
		Depth:           0,
		Context:         "context",
		LLMQuery:        func(_ context.Context, _ QueryRequest) (string, error) { return "", nil },
		RLMQuery:        func(_ context.Context, _ QueryRequest) (string, error) { return "", nil },
		LLMQueryBatched: llmBatch,
		RLMQueryBatched: rlmBatch,
	})
	if err != nil {
		t.Fatalf("expected session creation success, got %v", err)
	}
	return session
}

func mustUUIDv7String(t *testing.T) string {
	t.Helper()

	identifier, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("expected UUIDv7 generation success, got %v", err)
	}
	return identifier.String()
}
