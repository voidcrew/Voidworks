package ship

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// Replace just the literal name, preserving comments and custom registrations.
func rewriteName(data []byte, typePath, expected, name string) ([]byte, error) {
	return rewriteTextField(data, typePath, "name", expected, name)
}

// The field must be a known plain text variable (such as name, desc or slot).
func rewriteTextField(data []byte, typePath, field, expected, value string) ([]byte, error) {
	mask := dmSourceMask(data, true)
	start, end, matches := -1, len(data), 0
	for _, loc := range dmTypeLine.FindAllIndex(mask, -1) {
		if strings.TrimSpace(string(mask[loc[0]:loc[1]])) == typePath {
			start = loc[1]
			matches++
		} else if start >= 0 && end == len(data) {
			end = loc[0]
		}
	}
	if matches != 1 || end < start {
		return nil, fmt.Errorf("cannot uniquely locate %s in %s", field, typePath)
	}
	block := mask[start:end]
	if regexp.MustCompile(`(?m)^[\t ]*#`).Match(block) {
		return nil, fmt.Errorf("%s has conditional definitions; edit its %s in code", typePath, field)
	}
	assignments := textFieldAssignments(block, field)
	if len(assignments) > 1 {
		return nil, fmt.Errorf("multiple %s assignments in %s", field, typePath)
	}
	if len(assignments) == 0 {
		newline := "\n"
		if bytes.Contains(data, []byte("\r\n")) {
			newline = "\r\n"
		}
		insert := start
		if insert < len(data) && data[insert] == '\r' {
			insert++
		}
		if insert < len(data) && data[insert] == '\n' {
			insert++
		}
		assignment := "\t" + field + " = " + dmQuote(value) + newline
		if insert == start {
			assignment = newline + assignment
		}
		return append(append(append([]byte{}, data[:insert]...), assignment...), data[insert:]...), nil
	}
	a := start + assignments[0][1]
	for a < end && (data[a] == ' ' || data[a] == '\t') {
		a++
	}
	if a >= end || data[a] != '"' {
		return nil, fmt.Errorf("%s needs a literal %s to edit it here", typePath, field)
	}
	b := a + 1
	for b < end && data[b] != '"' {
		if data[b] == '\\' {
			b++
		}
		b++
	}
	if b >= end {
		return nil, fmt.Errorf("unterminated %s in %s", field, typePath)
	}
	b++
	lineEnd := b
	for lineEnd < end && mask[lineEnd] != '\n' {
		lineEnd++
	}
	if strings.TrimSpace(string(mask[b:lineEnd])) != "" {
		return nil, fmt.Errorf("%s uses a computed %s; edit it in code", typePath, field)
	}
	// DM joins backslash-newline continuations and discards their indentation.
	literal := regexp.MustCompile(`\\\r?\n[\t ]*`).ReplaceAllString(string(data[a:b]), "")
	current, err := dmUnquote(literal)
	if err != nil || current != expected {
		return nil, fmt.Errorf("%s %s differs from the loaded environment; reload the environment first", typePath, field)
	}
	return append(append(append([]byte{}, data[:a]...), dmQuote(value)...), data[b:]...), nil
}

// Named list entries (notably crew job names) are not datum assignments.
// The caller masks strings and comments first, preserving byte offsets.
func textFieldAssignments(block []byte, field string) [][]int {
	matches := regexp.MustCompile(`(?m)^[\t ]+`+regexp.QuoteMeta(field)+`[\t ]*=`).FindAllIndex(block, -1)
	var result [][]int
	pos, depth := 0, 0
	for _, match := range matches {
		for ; pos < match[0]; pos++ {
			switch block[pos] {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			}
		}
		if depth == 0 {
			result = append(result, match)
		}
	}
	return result
}
