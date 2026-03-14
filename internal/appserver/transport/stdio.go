package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
)

// LineHandler handles one inbound JSON line and optionally returns one response line.
type LineHandler func(context.Context, []byte) ([]byte, error)

// ServeStdioJSONL serves one line-delimited JSON stream over stdio-style streams.
func ServeStdioJSONL(ctx context.Context, reader io.Reader, writer io.Writer, maxLineBytes int, handler LineHandler) error {
	if reader == nil {
		return fmt.Errorf("reader is required")
	}
	if writer == nil {
		return fmt.Errorf("writer is required")
	}
	if maxLineBytes < 1 {
		return fmt.Errorf("maxLineBytes must be >= 1")
	}
	if handler == nil {
		return fmt.Errorf("handler is required")
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, min(maxLineBytes, 64*1024)), maxLineBytes)
	bufferedWriter := bufio.NewWriter(writer)
	defer bufferedWriter.Flush()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		response, err := handler(ctx, append([]byte(nil), scanner.Bytes()...))
		if err != nil {
			return err
		}
		if len(response) == 0 {
			continue
		}
		if _, err := bufferedWriter.Write(response); err != nil {
			return fmt.Errorf("failed to write response line; %w", err)
		}
		if err := bufferedWriter.WriteByte('\n'); err != nil {
			return fmt.Errorf("failed to terminate response line; %w", err)
		}
		if err := bufferedWriter.Flush(); err != nil {
			return fmt.Errorf("failed to flush response line; %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read request line; %w", err)
	}

	return nil
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
