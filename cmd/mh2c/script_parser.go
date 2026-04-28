package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func parseScriptFile(path string) (scriptFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return scriptFile{}, err
	}
	return parseScript(string(src))
}

func parseScript(src string) (scriptFile, error) {
	lines := strings.Split(src, "\n")
	out := scriptFile{}
	var current scriptTable

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(stripScriptComment(lines[i]))
		if line == "" {
			continue
		}

		switch line {
		case "[connection]":
			out.connection = scriptTable{}
			current = out.connection
			continue
		case "[[action]]":
			current = scriptTable{}
			out.actions = append(out.actions, current)
			continue
		}

		if current == nil {
			return scriptFile{}, fmt.Errorf("line %d: key/value must be inside [connection] or [[action]]", i+1)
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return scriptFile{}, fmt.Errorf("line %d: invalid assignment %q", i+1, line)
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if key == "" {
			return scriptFile{}, fmt.Errorf("line %d: empty key", i+1)
		}

		if strings.HasPrefix(rawValue, "[") && !hasBalancedBrackets(rawValue) {
			for {
				i++
				if i >= len(lines) {
					return scriptFile{}, fmt.Errorf("key %q: unterminated array", key)
				}
				next := strings.TrimSpace(stripScriptComment(lines[i]))
				rawValue += " " + next
				if hasBalancedBrackets(rawValue) {
					break
				}
			}
		}

		value, err := parseScriptValue(rawValue)
		if err != nil {
			return scriptFile{}, fmt.Errorf("key %q: %w", key, err)
		}
		current[key] = value
	}

	if len(out.actions) == 0 {
		return scriptFile{}, fmt.Errorf("script does not contain any [[action]] entries")
	}
	return out, nil
}

func parseScriptValue(raw string) (scriptValue, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "true" || raw == "false":
		return scriptValue{kind: scriptBool, boolean: raw == "true"}, nil
	case strings.HasPrefix(raw, "\""):
		value, err := strconv.Unquote(raw)
		if err != nil {
			return scriptValue{}, err
		}
		return scriptValue{kind: scriptString, str: value}, nil
	case strings.HasPrefix(raw, "["):
		list, err := parseStringArray(raw)
		if err != nil {
			return scriptValue{}, err
		}
		return scriptValue{kind: scriptStringList, list: list}, nil
	default:
		value, err := strconv.ParseInt(raw, 0, 64)
		if err != nil {
			return scriptValue{}, fmt.Errorf("unsupported value %q", raw)
		}
		return scriptValue{kind: scriptNumber, number: value}, nil
	}
}

func parseStringArray(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("invalid array %q", raw)
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return nil, nil
	}

	var out []string
	for len(inner) > 0 {
		inner = strings.TrimSpace(inner)
		if inner == "" {
			break
		}
		if inner[0] != '"' {
			return nil, fmt.Errorf("array elements must be strings")
		}
		end := 1
		escaped := false
		for end < len(inner) {
			ch := inner[end]
			if ch == '\\' && !escaped {
				escaped = true
				end++
				continue
			}
			if ch == '"' && !escaped {
				break
			}
			escaped = false
			end++
		}
		if end >= len(inner) {
			return nil, fmt.Errorf("unterminated string in array")
		}
		value, err := strconv.Unquote(inner[:end+1])
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		inner = strings.TrimSpace(inner[end+1:])
		if inner == "" {
			break
		}
		if inner[0] != ',' {
			return nil, fmt.Errorf("array elements must be separated by commas")
		}
		inner = inner[1:]
	}
	return out, nil
}

func hasBalancedBrackets(raw string) bool {
	depth := 0
	inString := false
	escaped := false

	for _, r := range raw {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case !inString && r == '[':
			depth++
		case !inString && r == ']':
			depth--
		}
	}
	return depth == 0 && !inString
}

func stripScriptComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return line[:i]
		}
	}
	return line
}
