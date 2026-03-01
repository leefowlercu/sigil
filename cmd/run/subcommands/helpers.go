package subcommands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func validateNoArgs(cmd *cobra.Command, args []string) error {
	return cobra.NoArgs(cmd, args)
}

func resolvePathOrDefault(path string, fallback string) string {
	if path == "" {
		return fallback
	}

	return path
}

func validateReadableRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path %q; %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to open file %q; %w", path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file %q; %w", path, err)
	}

	return nil
}

func validateReadableRegularFileIfExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to stat path %q; %w", path, err)
	}

	return validateReadableRegularFile(path)
}
