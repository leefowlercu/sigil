package repl

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func extractImports(code string) ([]string, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, nil
	}

	declarations := findImportDecls(trimmed)
	imports := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, declaration := range declarations {
		for _, importPath := range declaration.paths {
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
	declarations := findImportDecls(code)
	if len(declarations) == 0 {
		return strings.TrimSpace(code)
	}

	var builder strings.Builder
	last := 0
	for _, declaration := range declarations {
		if declaration.start < last || declaration.start > len(code) {
			continue
		}
		builder.WriteString(code[last:declaration.start])
		last = declaration.end
	}
	if last < len(code) {
		builder.WriteString(code[last:])
	}

	return strings.TrimSpace(builder.String())
}

type importDecl struct {
	start int
	end   int
	paths []string
}

func findImportDecls(code string) []importDecl {
	declarations := make([]importDecl, 0, 2)
	for index := 0; index < len(code); {
		next, ok := nextImportKeyword(code, index)
		if !ok {
			break
		}
		declaration, ok := parseImportDecl(code, next)
		if !ok {
			index = next + len("import")
			continue
		}
		declarations = append(declarations, declaration)
		index = declaration.end
	}
	return declarations
}

func nextImportKeyword(code string, start int) (int, bool) {
	for index := start; index < len(code); {
		switch code[index] {
		case '"':
			next, ok := skipDoubleQuoted(code, index)
			if !ok {
				return -1, false
			}
			index = next
		case '\'':
			next, ok := skipSingleQuoted(code, index)
			if !ok {
				return -1, false
			}
			index = next
		case '`':
			next, ok := skipRawString(code, index)
			if !ok {
				return -1, false
			}
			index = next
		case '/':
			if index+1 >= len(code) {
				index++
				continue
			}
			if code[index+1] == '/' {
				next := skipLineComment(code, index)
				index = next
				continue
			}
			if code[index+1] == '*' {
				next, ok := skipBlockComment(code, index)
				if !ok {
					return -1, false
				}
				index = next
				continue
			}
			index++
		default:
			if hasTokenAt(code, index, "import") {
				return index, true
			}
			_, size := utf8.DecodeRuneInString(code[index:])
			if size <= 0 {
				index++
				continue
			}
			index += size
		}
	}

	return -1, false
}

func parseImportDecl(code string, start int) (importDecl, bool) {
	cursor := start + len("import")
	cursor = skipHorizontalWhitespace(code, cursor)
	if cursor >= len(code) {
		return importDecl{}, false
	}

	if code[cursor] == '(' {
		end, paths, ok := parseImportBlock(code, cursor)
		if !ok {
			return importDecl{}, false
		}
		end = consumeOptionalTerminator(code, end)
		return importDecl{start: start, end: end, paths: paths}, true
	}

	path, end, ok := parseImportSpec(code, cursor)
	if !ok {
		return importDecl{}, false
	}
	end = consumeImportSingleEnd(code, end)
	end = consumeOptionalTerminator(code, end)
	paths := make([]string, 0, 1)
	if path != "" {
		paths = append(paths, path)
	}
	return importDecl{start: start, end: end, paths: paths}, true
}

func parseImportBlock(code string, openParen int) (int, []string, bool) {
	cursor := openParen + 1
	depth := 1
	for cursor < len(code) {
		switch code[cursor] {
		case '"':
			next, ok := skipDoubleQuoted(code, cursor)
			if !ok {
				return 0, nil, false
			}
			cursor = next
		case '\'':
			next, ok := skipSingleQuoted(code, cursor)
			if !ok {
				return 0, nil, false
			}
			cursor = next
		case '`':
			next, ok := skipRawString(code, cursor)
			if !ok {
				return 0, nil, false
			}
			cursor = next
		case '/':
			if cursor+1 >= len(code) {
				return 0, nil, false
			}
			if code[cursor+1] == '/' {
				cursor = skipLineComment(code, cursor)
				continue
			}
			if code[cursor+1] == '*' {
				next, ok := skipBlockComment(code, cursor)
				if !ok {
					return 0, nil, false
				}
				cursor = next
				continue
			}
			cursor++
		case '(':
			depth++
			cursor++
		case ')':
			depth--
			cursor++
			if depth == 0 {
				body := code[openParen+1 : cursor-1]
				return cursor, parseImportPaths(body), true
			}
		default:
			_, size := utf8.DecodeRuneInString(code[cursor:])
			if size <= 0 {
				cursor++
				continue
			}
			cursor += size
		}
	}

	return 0, nil, false
}

func parseImportPaths(body string) []string {
	paths := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for cursor := 0; cursor < len(body); {
		switch body[cursor] {
		case '"':
			literal, next, ok := readDoubleQuotedLiteral(body, cursor)
			if !ok {
				return paths
			}
			path, err := strconv.Unquote(literal)
			if err == nil {
				if _, exists := seen[path]; !exists {
					seen[path] = struct{}{}
					paths = append(paths, path)
				}
			}
			cursor = next
		case '\'':
			next, ok := skipSingleQuoted(body, cursor)
			if !ok {
				return paths
			}
			cursor = next
		case '`':
			next, ok := skipRawString(body, cursor)
			if !ok {
				return paths
			}
			cursor = next
		case '/':
			if cursor+1 >= len(body) {
				cursor++
				continue
			}
			if body[cursor+1] == '/' {
				cursor = skipLineComment(body, cursor)
				continue
			}
			if body[cursor+1] == '*' {
				next, ok := skipBlockComment(body, cursor)
				if !ok {
					return paths
				}
				cursor = next
				continue
			}
			cursor++
		default:
			_, size := utf8.DecodeRuneInString(body[cursor:])
			if size <= 0 {
				cursor++
				continue
			}
			cursor += size
		}
	}
	return paths
}

func parseImportSpec(code string, start int) (string, int, bool) {
	cursor := skipHorizontalWhitespace(code, start)
	if cursor >= len(code) {
		return "", 0, false
	}
	if code[cursor] == '.' || code[cursor] == '_' {
		cursor++
		cursor = skipHorizontalWhitespace(code, cursor)
	} else if isIdentifierStart(code[cursor]) {
		next := cursor + 1
		for next < len(code) && isIdentifierPart(code[next]) {
			next++
		}
		cursor = skipHorizontalWhitespace(code, next)
	}

	if cursor >= len(code) || code[cursor] != '"' {
		return "", 0, false
	}
	literal, next, ok := readDoubleQuotedLiteral(code, cursor)
	if !ok {
		return "", 0, false
	}
	path, err := strconv.Unquote(literal)
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(path), next, true
}

func consumeImportSingleEnd(code string, start int) int {
	for cursor := start; cursor < len(code); {
		switch code[cursor] {
		case ';':
			return cursor + 1
		case '\n':
			return cursor
		case '/':
			if cursor+1 >= len(code) {
				return cursor + 1
			}
			if code[cursor+1] == '/' {
				return skipLineComment(code, cursor)
			}
			if code[cursor+1] == '*' {
				next, ok := skipBlockComment(code, cursor)
				if !ok {
					return len(code)
				}
				cursor = next
				continue
			}
			cursor++
		default:
			_, size := utf8.DecodeRuneInString(code[cursor:])
			if size <= 0 {
				return cursor + 1
			}
			cursor += size
		}
	}
	return len(code)
}

func consumeOptionalTerminator(code string, start int) int {
	cursor := start
	for cursor < len(code) {
		r, size := utf8.DecodeRuneInString(code[cursor:])
		if size <= 0 {
			return cursor
		}
		if r == ';' {
			return cursor + size
		}
		if !unicode.IsSpace(r) {
			return cursor
		}
		cursor += size
	}
	return cursor
}

func skipHorizontalWhitespace(code string, start int) int {
	cursor := start
	for cursor < len(code) {
		r, size := utf8.DecodeRuneInString(code[cursor:])
		if size <= 0 {
			return cursor
		}
		if r == '\n' || !unicode.IsSpace(r) {
			return cursor
		}
		cursor += size
	}
	return cursor
}

func hasTokenAt(code string, index int, token string) bool {
	if index < 0 || index+len(token) > len(code) {
		return false
	}
	if code[index:index+len(token)] != token {
		return false
	}
	if index > 0 && isIdentifierPart(code[index-1]) {
		return false
	}
	if index+len(token) < len(code) && isIdentifierPart(code[index+len(token)]) {
		return false
	}
	return true
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isIdentifierPart(ch byte) bool {
	return isIdentifierStart(ch) || ('0' <= ch && ch <= '9')
}

func readDoubleQuotedLiteral(input string, start int) (string, int, bool) {
	if start >= len(input) || input[start] != '"' {
		return "", 0, false
	}
	cursor := start + 1
	for cursor < len(input) {
		switch input[cursor] {
		case '\\':
			cursor += 2
		case '"':
			return input[start : cursor+1], cursor + 1, true
		default:
			_, size := utf8.DecodeRuneInString(input[cursor:])
			if size <= 0 {
				return "", 0, false
			}
			cursor += size
		}
	}
	return "", 0, false
}

func skipDoubleQuoted(input string, start int) (int, bool) {
	_, next, ok := readDoubleQuotedLiteral(input, start)
	return next, ok
}

func skipSingleQuoted(input string, start int) (int, bool) {
	if start >= len(input) || input[start] != '\'' {
		return 0, false
	}
	cursor := start + 1
	for cursor < len(input) {
		switch input[cursor] {
		case '\\':
			cursor += 2
		case '\'':
			return cursor + 1, true
		default:
			_, size := utf8.DecodeRuneInString(input[cursor:])
			if size <= 0 {
				return 0, false
			}
			cursor += size
		}
	}
	return 0, false
}

func skipRawString(input string, start int) (int, bool) {
	if start >= len(input) || input[start] != '`' {
		return 0, false
	}
	cursor := start + 1
	for cursor < len(input) {
		if input[cursor] == '`' {
			return cursor + 1, true
		}
		cursor++
	}
	return 0, false
}

func skipLineComment(input string, start int) int {
	cursor := start
	for cursor < len(input) {
		if input[cursor] == '\n' {
			return cursor
		}
		cursor++
	}
	return len(input)
}

func skipBlockComment(input string, start int) (int, bool) {
	if start+1 >= len(input) || input[start] != '/' || input[start+1] != '*' {
		return 0, false
	}
	cursor := start + 2
	for cursor+1 < len(input) {
		if input[cursor] == '*' && input[cursor+1] == '/' {
			return cursor + 2, true
		}
		cursor++
	}
	return 0, false
}
