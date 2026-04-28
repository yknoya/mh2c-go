package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func parseScriptFile(path string) (scriptFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return scriptFile{}, err
	}
	return parseScript(string(src))
}

func parseScript(src string) (scriptFile, error) {
	var raw struct {
		Connection map[string]any   `toml:"connection"`
		Actions    []map[string]any `toml:"action"`
	}
	if err := toml.NewDecoder(strings.NewReader(src)).DisallowUnknownFields().Decode(&raw); err != nil {
		return scriptFile{}, err
	}
	if len(raw.Actions) == 0 {
		return scriptFile{}, fmt.Errorf("script does not contain any [[action]] entries")
	}

	connection, err := convertScriptTable("connection", raw.Connection)
	if err != nil {
		return scriptFile{}, err
	}
	actions := make([]scriptTable, 0, len(raw.Actions))
	for i, action := range raw.Actions {
		table, err := convertScriptTable(fmt.Sprintf("action %d", i+1), action)
		if err != nil {
			return scriptFile{}, err
		}
		actions = append(actions, table)
	}
	return scriptFile{connection: connection, actions: actions}, nil
}

func convertScriptTable(scope string, raw map[string]any) (scriptTable, error) {
	out := make(scriptTable, len(raw))
	for key, value := range raw {
		converted, err := convertScriptValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", scope, key, err)
		}
		out[key] = converted
	}
	return out, nil
}

func convertScriptValue(value any) (scriptValue, error) {
	switch typed := value.(type) {
	case string:
		return scriptValue{kind: scriptString, str: typed}, nil
	case int64:
		return scriptValue{kind: scriptNumber, number: typed}, nil
	case bool:
		return scriptValue{kind: scriptBool, boolean: typed}, nil
	case []any:
		list := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return scriptValue{}, fmt.Errorf("array elements must be strings")
			}
			list = append(list, text)
		}
		return scriptValue{kind: scriptStringList, list: list}, nil
	default:
		return scriptValue{}, fmt.Errorf("unsupported TOML value type %T", value)
	}
}
