package appserver

import (
	"bufio"
	"fmt"
	"io"
	"sync"
)

type stdioJSONRPCWriter struct {
	writer *bufio.Writer
	mu     sync.Mutex
}

func newStdioJSONRPCWriter(writer io.Writer) *stdioJSONRPCWriter {
	return &stdioJSONRPCWriter{
		writer: bufio.NewWriter(writer),
	}
}

func (w *stdioJSONRPCWriter) writeText(message []byte) error {
	if w == nil || w.writer == nil {
		return fmt.Errorf("stdio writer is not configured")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.writer.Write(message); err != nil {
		return err
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return err
	}
	return w.writer.Flush()
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
