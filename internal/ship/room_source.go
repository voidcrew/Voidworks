package ship

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Mask comments (and optionally strings) without changing source offsets.
// This lets the small slot-list edit ignore examples in comments and strings.
func dmSourceMask(data []byte, maskStrings bool) []byte {
	out := append([]byte{}, data...)
	quote, block, line := byte(0), 0, false
	blank := func(i int) {
		if out[i] != '\n' && out[i] != '\r' {
			out[i] = ' '
		}
	}
	for i := 0; i < len(data); i++ {
		ch := data[i]
		next := byte(0)
		if i+1 < len(data) {
			next = data[i+1]
		}
		if line {
			blank(i)
			if ch == '\n' {
				line = false
			}
			continue
		}
		if block > 0 {
			blank(i)
			if ch == '/' && next == '*' {
				block++
				i++
				blank(i)
			} else if ch == '*' && next == '/' {
				block--
				i++
				blank(i)
			}
			continue
		}
		if quote != 0 {
			if maskStrings {
				blank(i)
			}
			if ch == '\\' && i+1 < len(data) {
				i++
				if maskStrings {
					blank(i)
				}
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '/' && (next == '/' || next == '*') {
			line = next == '/'
			if !line {
				block = 1
			}
			blank(i)
			i++
			blank(i)
		} else if ch == '"' || ch == '\'' {
			quote = ch
			if maskStrings {
				blank(i)
			}
		}
	}
	return out
}

var dmTypeLine = regexp.MustCompile(`(?m)^/[^\r\n]*`)
var roomSlotsAssignment = regexp.MustCompile(`(?m)^[\t ]+upgrade_slot_ids[\t ]*=[\t ]*`)

// dmTypeBlock finds the one definition block of typePath in masked source,
// returning the offsets just after its type line and before the next one.
func dmTypeBlock(data, mask []byte, typePath, what string) (int, int, error) {
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
		return 0, 0, fmt.Errorf("cannot uniquely locate the %s in %s", what, typePath)
	}
	if regexp.MustCompile(`(?m)^[\t ]*#`).Match(mask[start:end]) {
		return 0, 0, fmt.Errorf("%s has conditional definitions; use a literal %s", typePath, what)
	}
	return start, end, nil
}

// insertDefinitionLine adds one indented assignment as the first line of a
// type block that starts at offset start.
func insertDefinitionLine(data []byte, start int, line string) []byte {
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
	assignment := "\t" + line + newline
	if insert == start {
		assignment = newline + assignment
	}
	return append(append(append([]byte{}, data[:insert]...), []byte(assignment)...), data[insert:]...)
}

// Rewrite one literal slot list in its actual type block. Everything outside
// the expression, including jobs, prices, inheritance and procedures, is kept.
func rewriteRoomSlots(data []byte, typePath string, expected, slots []string) ([]byte, error) {
	mask := dmSourceMask(data, true)
	start, end, err := dmTypeBlock(data, mask, typePath, "upgrade_slot_ids list")
	if err != nil {
		return nil, err
	}
	block := mask[start:end]
	assignments := roomSlotsAssignment.FindAllIndex(block, -1)
	if len(assignments) > 1 {
		return nil, fmt.Errorf("multiple module lists in %s", typePath)
	}
	if len(assignments) == 0 {
		if reflect.DeepEqual(expected, slots) {
			return append([]byte{}, data...), nil
		}
		return insertDefinitionLine(data, start, "upgrade_slot_ids = "+dmList(slots)), nil
	}
	valueStart := start + assignments[0][1]
	valueEnd := valueStart
	if bytes.HasPrefix(mask[valueStart:], []byte("list(")) {
		depth := 0
		for ; valueEnd < end; valueEnd++ {
			if mask[valueEnd] == '(' {
				depth++
			}
			if mask[valueEnd] == ')' {
				depth--
				if depth == 0 {
					valueEnd++
					break
				}
			}
		}
		if depth != 0 {
			return nil, fmt.Errorf("unterminated module list in %s", typePath)
		}
	} else if bytes.HasPrefix(mask[valueStart:], []byte("null")) {
		valueEnd += 4
	} else {
		return nil, fmt.Errorf("%s needs a literal upgrade_slot_ids list before adding modules", typePath)
	}
	lineEnd := valueEnd
	for lineEnd < end && mask[lineEnd] != '\n' {
		lineEnd++
	}
	if strings.TrimSpace(string(mask[valueEnd:lineEnd])) != "" {
		return nil, fmt.Errorf("%s uses an expression for upgrade_slot_ids; use a literal list", typePath)
	}
	current, err := stringList(string(dmSourceMask(data[valueStart:valueEnd], false)))
	if err != nil || !reflect.DeepEqual(current, expected) {
		return nil, fmt.Errorf("%s module list differs from the loaded environment; reload the environment first", typePath)
	}
	if reflect.DeepEqual(current, slots) {
		return append([]byte{}, data...), nil
	}
	return append(append(append([]byte{}, data[:valueStart]...), []byte(dmList(slots))...), data[valueEnd:]...), nil
}

// Enable upgrade slots on a fixed ship's own definition. An inherited or FALSE
// value gains an explicit TRUE beside the slot list; TRUE is left untouched.
func rewriteModularFlag(data []byte, typePath string) ([]byte, error) {
	return rewriteFlag(data, typePath, "has_upgrade_slots", true)
}

// rewriteFlag sets one literal TRUE/FALSE field in a type block, inserting
// the assignment when the block inherits its value.
func rewriteFlag(data []byte, typePath, field string, value bool) ([]byte, error) {
	target := "FALSE"
	if value {
		target = "TRUE"
	}
	mask := dmSourceMask(data, true)
	start, end, err := dmTypeBlock(data, mask, typePath, field+" value")
	if err != nil {
		return nil, err
	}
	assignments := regexp.MustCompile(`(?m)^[\t ]+`+regexp.QuoteMeta(field)+`[\t ]*=[\t ]*`).FindAllIndex(mask[start:end], -1)
	if len(assignments) > 1 {
		return nil, fmt.Errorf("multiple %s values in %s", field, typePath)
	}
	if len(assignments) == 0 {
		return insertDefinitionLine(data, start, field+" = "+target), nil
	}
	valueStart := start + assignments[0][1]
	valueEnd, lineEnd := valueStart, valueStart
	for lineEnd < end && mask[lineEnd] != '\n' && mask[lineEnd] != '\r' {
		lineEnd++
	}
	for valueEnd < lineEnd && mask[valueEnd] != ' ' && mask[valueEnd] != '\t' {
		valueEnd++
	}
	// Comments are masked already, so anything left after the value is code.
	if strings.TrimSpace(string(mask[valueEnd:lineEnd])) == "" {
		switch current := string(mask[valueStart:valueEnd]); current {
		case "TRUE", "1", "FALSE", "0":
			if (current == "TRUE" || current == "1") == value {
				return append([]byte{}, data...), nil
			}
			return append(append(append([]byte{}, data[:valueStart]...), []byte(target)...), data[valueEnd:]...), nil
		}
	}
	return nil, fmt.Errorf("%s uses an expression for %s; use TRUE or FALSE", typePath, field)
}
