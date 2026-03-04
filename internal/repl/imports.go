package repl

import (
	"regexp"
	"strings"
)

var (
	singleImportPattern = regexp.MustCompile(`(?m)^\s*import\s+(?:(?:[A-Za-z_][A-Za-z0-9_]*|\.)\s+)?"([^"]+)"\s*;?`)
	blockImportPattern  = regexp.MustCompile(`(?s)import\s*\((.*?)\)\s*;?`)
	stringLiteralRegex  = regexp.MustCompile(`"([^"]+)"`)
)

func extractImports(code string) ([]string, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, nil
	}

	imports := make([]string, 0, 4)
	seen := make(map[string]struct{})

	singleMatches := singleImportPattern.FindAllStringSubmatch(trimmed, -1)
	for _, match := range singleMatches {
		if len(match) != 2 {
			continue
		}
		importPath := strings.TrimSpace(match[1])
		if importPath == "" {
			continue
		}
		if _, ok := seen[importPath]; ok {
			continue
		}
		seen[importPath] = struct{}{}
		imports = append(imports, importPath)
	}

	blockMatches := blockImportPattern.FindAllStringSubmatch(trimmed, -1)
	for _, blockMatch := range blockMatches {
		if len(blockMatch) != 2 {
			continue
		}
		for _, literal := range stringLiteralRegex.FindAllStringSubmatch(blockMatch[1], -1) {
			if len(literal) != 2 {
				continue
			}
			importPath := strings.TrimSpace(literal[1])
			if importPath == "" {
				continue
			}
			if _, ok := seen[importPath]; ok {
				continue
			}
			seen[importPath] = struct{}{}
			imports = append(imports, importPath)
		}
	}

	return imports, nil
}

func stripImportDecls(code string) string {
	withoutBlocks := blockImportPattern.ReplaceAllString(code, "")
	withoutSingles := singleImportPattern.ReplaceAllString(withoutBlocks, "")
	return strings.TrimSpace(withoutSingles)
}
