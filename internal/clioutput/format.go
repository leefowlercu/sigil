package clioutput

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Format identifies the user-facing CLI output mode.
type Format string

const (
	// FormatText renders human-readable output.
	FormatText Format = "text"
	// FormatJSON renders machine-readable JSON output.
	FormatJSON Format = "json"
)

const outputFlagUsage = "Output format (text|json)"

// ParseFormat validates one output format value.
func ParseFormat(raw string) (Format, error) {
	switch Format(strings.TrimSpace(raw)) {
	case FormatText:
		return FormatText, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("output must be one of: text, json")
	}
}

// AddOutputFlag binds the canonical output-format flag to a Cobra command.
func AddOutputFlag(cmd *cobra.Command, target *Format) {
	if cmd == nil || target == nil {
		return
	}

	*target = FormatText
	cmd.PersistentFlags().VarP(newFormatValue(target), "output", "o", outputFlagUsage)
}

// ResolveFormat returns the validated output format for a command tree.
func ResolveFormat(cmd *cobra.Command) Format {
	for current := cmd; current != nil; current = current.Parent() {
		flag := current.Flags().Lookup("output")
		if flag == nil {
			flag = current.PersistentFlags().Lookup("output")
		}
		if flag == nil {
			continue
		}

		format, err := ParseFormat(flag.Value.String())
		if err == nil {
			return format
		}
	}

	return FormatText
}

type formatValue struct {
	target *Format
}

func newFormatValue(target *Format) *formatValue {
	return &formatValue{target: target}
}

func (v *formatValue) String() string {
	if v == nil || v.target == nil {
		return string(FormatText)
	}

	if *v.target == "" {
		return string(FormatText)
	}

	return string(*v.target)
}

func (v *formatValue) Set(raw string) error {
	if v == nil || v.target == nil {
		return fmt.Errorf("output format target is required")
	}

	format, err := ParseFormat(raw)
	if err != nil {
		return err
	}

	*v.target = format
	return nil
}

func (v *formatValue) Type() string {
	return "output-format"
}
