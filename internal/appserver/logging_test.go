package appserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type testLogSink struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *testLogSink) Write(payload []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buffer.Write(payload)
}

func (s *testLogSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buffer.String()
}

func captureTestLogs(t *testing.T) *testLogSink {
	t.Helper()

	sink := &testLogSink{}
	previous := slog.Default()
	logger := slog.New(slog.NewJSONHandler(sink, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return sink
}

func waitForTestLogRecord(t *testing.T, sink *testLogSink, match func(map[string]any) bool) map[string]any {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, record := range decodeTestLogRecords(t, sink) {
			if match(record) {
				return record
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for log record; logs=%s", sink.String())
	return nil
}

func decodeTestLogRecords(t *testing.T, sink *testLogSink) []map[string]any {
	t.Helper()

	rawLines := bytes.Split([]byte(sink.String()), []byte("\n"))
	records := make([]map[string]any, 0, len(rawLines))
	for _, rawLine := range rawLines {
		if len(bytes.TrimSpace(rawLine)) == 0 {
			continue
		}

		record := map[string]any{}
		if err := json.Unmarshal(rawLine, &record); err != nil {
			t.Fatalf("expected valid log json, got %v for %q", err, string(rawLine))
		}
		records = append(records, record)
	}
	return records
}
