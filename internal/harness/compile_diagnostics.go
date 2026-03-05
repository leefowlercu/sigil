package harness

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/leefowlercu/sigil/internal/repl"
)

var (
	compileLocationPattern = regexp.MustCompile(`(?m)(?:^|[\r\n])(?:[^:\n]+:)?([0-9]+):([0-9]+):\s*(.+)$`)
	undefinedSymbolPattern = regexp.MustCompile(`undefined:\s*([A-Za-z0-9_\.]+)`)
)

func compileErrorDetail(execErr error, stderr string) *ActionErrorDetail {
	if !repl.IsCode(execErr, repl.ErrorCodeExecutionCompile) {
		return nil
	}

	message := strings.TrimSpace(execErr.Error())
	var typed *repl.Error
	if errors.As(execErr, &typed) {
		if typed.Cause != nil && strings.TrimSpace(typed.Cause.Error()) != "" {
			message = strings.TrimSpace(typed.Cause.Error())
		} else if strings.TrimSpace(typed.Message) != "" {
			message = strings.TrimSpace(typed.Message)
		}
	}

	detail := &ActionErrorDetail{
		Stage:   "compile",
		Message: message,
	}

	matches := compileLocationPattern.FindStringSubmatch(message)
	if len(matches) == 4 {
		if line, err := strconv.Atoi(matches[1]); err == nil && line >= 0 {
			detail.Line = &line
		}
		if column, err := strconv.Atoi(matches[2]); err == nil && column >= 0 {
			detail.Column = &column
		}
		diagnosticMessage := strings.TrimSpace(matches[3])
		if diagnosticMessage != "" {
			detail.Message = diagnosticMessage
		}
	}

	if symbolMatches := undefinedSymbolPattern.FindStringSubmatch(detail.Message); len(symbolMatches) == 2 {
		symbol := strings.TrimSpace(symbolMatches[1])
		if symbol != "" {
			detail.Symbol = &symbol
		}
	}

	if detail.Line != nil {
		if sourceLine := sourceLineAt(stderr, *detail.Line); sourceLine != "" {
			detail.SourceLine = &sourceLine
		}
	}

	return detail
}

func sourceLineAt(stderr string, line int) string {
	if line < 1 {
		return ""
	}

	normalized := strings.ReplaceAll(stderr, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if line > len(lines) {
		return ""
	}

	candidate := strings.TrimSpace(lines[line-1])
	if candidate == "" {
		return ""
	}

	return candidate
}
