package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type scriptFieldSpec struct {
	name     string
	kind     scriptValueKind
	required bool
}

type scriptActionSpec struct {
	name        string
	fields      []scriptFieldSpec
	flags       []string
	settings    []string
	untilValues []string
	notes       []string
}

var scriptConnectionFields = []scriptFieldSpec{
	{name: "url", kind: scriptString},
	{name: "scheme", kind: scriptString},
	{name: "host", kind: scriptString},
	{name: "authority", kind: scriptString},
	{name: "path", kind: scriptString},
	{name: "port", kind: scriptNumber},
	{name: "insecure", kind: scriptBool},
	{name: "send_goaway", kind: scriptBool},
	{name: "timeout_ms", kind: scriptNumber},
	{name: "max_table_size", kind: scriptNumber},
}

var scriptActionSpecs = []scriptActionSpec{
	{
		name: "preface",
	},
	{
		name:   "sleep",
		fields: []scriptFieldSpec{{name: "duration_ms", kind: scriptNumber, required: true}},
		notes:  []string{"duration_ms must be greater than 0."},
	},
	{
		name:     "settings",
		fields:   []scriptFieldSpec{{name: "settings", kind: scriptStringList}, {name: "flags", kind: scriptStringList}},
		flags:    sortedKeys(settingsFlagNames),
		settings: []string{"HEADER_TABLE_SIZE", "ENABLE_PUSH", "MAX_CONCURRENT_STREAMS", "INITIAL_WINDOW_SIZE", "MAX_FRAME_SIZE", "MAX_HEADER_LIST_SIZE"},
	},
	{
		name: "headers",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "flags", kind: scriptStringList},
			{name: "headers", kind: scriptStringList},
			{name: "block_hex", kind: scriptString},
			{name: "stream_dep", kind: scriptNumber},
			{name: "weight", kind: scriptNumber},
			{name: "exclusive", kind: scriptBool},
			{name: "pad_length", kind: scriptNumber},
		},
		flags: sortedKeys(headersFlagNames),
		notes: []string{"Exactly one of headers or block_hex is required.", "stream_dep and weight are required when flags contains PRIORITY."},
	},
	{
		name: "continuation",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "flags", kind: scriptStringList},
			{name: "block_hex", kind: scriptString, required: true},
		},
		flags: sortedKeys(continuationFlagNames),
	},
	{
		name: "data",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "flags", kind: scriptStringList},
			{name: "data", kind: scriptString},
			{name: "data_hex", kind: scriptString},
			{name: "pad_length", kind: scriptNumber},
		},
		flags: sortedKeys(dataFlagNames),
		notes: []string{"data and data_hex cannot be used together."},
	},
	{
		name: "ping",
		fields: []scriptFieldSpec{
			{name: "flags", kind: scriptStringList},
			{name: "data", kind: scriptString},
			{name: "data_hex", kind: scriptString},
		},
		flags: sortedKeys(pingFlagNames),
		notes: []string{"Exactly one of data or data_hex is required."},
	},
	{
		name: "goaway",
		fields: []scriptFieldSpec{
			{name: "last_stream_id", kind: scriptNumber, required: true},
			{name: "error_code", kind: scriptString},
			{name: "debug_data", kind: scriptString},
			{name: "debug_hex", kind: scriptString},
		},
		notes: []string{"debug_data and debug_hex cannot be used together."},
	},
	{
		name: "window_update",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "increment", kind: scriptNumber, required: true},
		},
	},
	{
		name: "rst_stream",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "error_code", kind: scriptString},
		},
	},
	{
		name: "priority",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "stream_dep", kind: scriptNumber, required: true},
			{name: "weight", kind: scriptNumber, required: true},
			{name: "exclusive", kind: scriptBool},
		},
	},
	{
		name: "push_promise",
		fields: []scriptFieldSpec{
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "promised_stream_id", kind: scriptNumber, required: true},
			{name: "flags", kind: scriptStringList},
			{name: "headers", kind: scriptStringList},
			{name: "block_hex", kind: scriptString},
			{name: "pad_length", kind: scriptNumber},
		},
		flags: sortedKeys(pushPromiseFlagNames),
		notes: []string{"Exactly one of headers or block_hex is required."},
	},
	{
		name: "raw",
		fields: []scriptFieldSpec{
			{name: "frame_type", kind: scriptNumber, required: true},
			{name: "stream_id", kind: scriptNumber, required: true},
			{name: "flags", kind: scriptNumber, required: true},
			{name: "payload_hex", kind: scriptString, required: true},
		},
	},
	{
		name: "receive",
		fields: []scriptFieldSpec{
			{name: "count", kind: scriptNumber},
			{name: "until", kind: scriptString},
			{name: "stream_id", kind: scriptNumber},
			{name: "ack_settings", kind: scriptBool},
			{name: "ack_ping", kind: scriptBool},
		},
		untilValues: []string{"settings", "settings_ack", "ping_ack", "goaway", "end_stream"},
		notes:       []string{"count and until cannot be used together.", "stream_id is required when until is end_stream."},
	},
}

func describeScriptActions(w io.Writer, typeFilter string) error {
	if typeFilter != "" {
		spec, ok := findScriptActionSpec(typeFilter)
		if !ok {
			return fmt.Errorf("unknown script action type %q", typeFilter)
		}
		return writeScriptActionDescription(w, spec)
	}

	if _, err := fmt.Fprintln(w, "Script action types:"); err != nil {
		return err
	}
	for _, spec := range scriptActionSpecs {
		if _, err := fmt.Fprintf(w, "  - %s\n", spec.name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Use `mh2c script describe --type <action_type>` for fields."); err != nil {
		return err
	}
	return nil
}

func writeScriptActionDescription(w io.Writer, spec scriptActionSpec) error {
	if _, err := fmt.Fprintf(w, "Action: %s\n", spec.name); err != nil {
		return err
	}
	if len(spec.fields) == 0 {
		if _, err := fmt.Fprintln(w, "Fields: none"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Fields:"); err != nil {
			return err
		}
		for _, field := range spec.fields {
			required := "optional"
			if field.required {
				required = "required"
			}
			if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", field.name, scriptKindName(field.kind), required); err != nil {
				return err
			}
		}
	}
	if len(spec.flags) > 0 {
		if _, err := fmt.Fprintf(w, "Flags: %s\n", strings.Join(spec.flags, ", ")); err != nil {
			return err
		}
	}
	if len(spec.settings) > 0 {
		if _, err := fmt.Fprintf(w, "Settings: %s\n", strings.Join(spec.settings, ", ")); err != nil {
			return err
		}
	}
	if len(spec.untilValues) > 0 {
		if _, err := fmt.Fprintf(w, "Until values: %s\n", strings.Join(spec.untilValues, ", ")); err != nil {
			return err
		}
	}
	if len(spec.notes) > 0 {
		if _, err := fmt.Fprintln(w, "Notes:"); err != nil {
			return err
		}
		for _, note := range spec.notes {
			if _, err := fmt.Fprintf(w, "  - %s\n", note); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeScriptTemplate(w io.Writer, name string) error {
	if name != "request" {
		return fmt.Errorf("unknown script template %q: want request", name)
	}
	_, err := io.WriteString(w, requestScriptTemplate)
	return err
}

func validateScript(script scriptFile) error {
	if err := validateScriptTable("connection", script.connection, scriptConnectionFields); err != nil {
		return err
	}
	for i, action := range script.actions {
		actionType, err := action.requireString("type")
		if err != nil {
			return fmt.Errorf("action %d: %w", i+1, err)
		}
		spec, ok := findScriptActionSpec(actionType)
		if !ok {
			return fmt.Errorf("action %d: unsupported action type %q", i+1, actionType)
		}
		if err := validateScriptAction(spec, action); err != nil {
			return fmt.Errorf("action %d: %w", i+1, err)
		}
	}
	return nil
}

func validateScriptAction(spec scriptActionSpec, action scriptTable) error {
	fields := append([]scriptFieldSpec{{name: "type", kind: scriptString, required: true}}, spec.fields...)
	if err := validateScriptTable("action", action, fields); err != nil {
		return err
	}

	switch spec.name {
	case "sleep":
		_, err := parseSleepDuration(action)
		return err
	case "settings":
		if settings, ok, err := action.stringListValue("settings"); err != nil {
			return err
		} else if ok {
			if _, err := parseScriptSettings(settings); err != nil {
				return err
			}
		}
		return parseFlagsOnly(action, settingsFlagNames)
	case "headers":
		if err := validateExactlyOne(action, "headers", "block_hex"); err != nil {
			return err
		}
		if err := validateHeaderList(action, "headers"); err != nil {
			return err
		}
		if err := validateHexField(action, "block_hex"); err != nil {
			return err
		}
		flags, err := parseFlags(action, headersFlagNames)
		if err != nil {
			return err
		}
		if flags&flagValue(headersFlagNames, "PRIORITY") != 0 {
			if _, err := parsePriority(action); err != nil {
				return err
			}
		}
	case "continuation":
		if err := validateHexField(action, "block_hex"); err != nil {
			return err
		}
		return parseFlagsOnly(action, continuationFlagNames)
	case "data":
		if err := validateNotBoth(action, "data", "data_hex"); err != nil {
			return err
		}
		if err := validateHexField(action, "data_hex"); err != nil {
			return err
		}
		return parseFlagsOnly(action, dataFlagNames)
	case "ping":
		if _, err := parsePingActionData(action); err != nil {
			return err
		}
		return parseFlagsOnly(action, pingFlagNames)
	case "goaway":
		if _, err := parseErrorCodeAction(action); err != nil {
			return err
		}
		if err := validateNotBoth(action, "debug_data", "debug_hex"); err != nil {
			return err
		}
		return validateHexField(action, "debug_hex")
	case "window_update":
		_, err := action.requireUint32("increment")
		return err
	case "rst_stream":
		_, err := parseErrorCodeAction(action)
		return err
	case "priority":
		_, err := parsePriority(action)
		return err
	case "push_promise":
		if err := validateExactlyOne(action, "headers", "block_hex"); err != nil {
			return err
		}
		if err := validateHeaderList(action, "headers"); err != nil {
			return err
		}
		if err := validateHexField(action, "block_hex"); err != nil {
			return err
		}
		return parseFlagsOnly(action, pushPromiseFlagNames)
	case "raw":
		if flags, ok, err := action.intValue("flags"); err != nil {
			return err
		} else if !ok || flags < 0 || flags > 0xff {
			return fmt.Errorf("flags must be set to 0..255 for raw frames")
		}
		return validateHexField(action, "payload_hex")
	case "receive":
		return validateReceiveAction(action)
	}
	return nil
}

func validateScriptTable(scope string, table scriptTable, fields []scriptFieldSpec) error {
	allowed := make(map[string]scriptFieldSpec, len(fields))
	for _, field := range fields {
		allowed[field.name] = field
	}
	for key, value := range table {
		field, ok := allowed[key]
		if !ok {
			return fmt.Errorf("%s.%s is not supported", scope, key)
		}
		if value.kind != field.kind {
			return fmt.Errorf("%s.%s must be %s", scope, key, scriptKindName(field.kind))
		}
	}
	for _, field := range fields {
		if field.required {
			if _, ok := table[field.name]; !ok {
				return fmt.Errorf("%s.%s is required", scope, field.name)
			}
		}
	}
	return nil
}

func validateReceiveAction(action scriptTable) error {
	if _, hasCount := action["count"]; hasCount {
		if _, hasUntil := action["until"]; hasUntil {
			return fmt.Errorf("count and until cannot be used together")
		}
	}
	if count, ok, err := action.intValue("count"); err != nil {
		return err
	} else if ok && count <= 0 {
		return fmt.Errorf("count must be > 0")
	}
	until, ok, err := action.stringValue("until")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	switch until {
	case "settings", "settings_ack", "ping_ack", "goaway":
		return nil
	case "end_stream":
		if _, ok := action["stream_id"]; !ok {
			return fmt.Errorf("stream_id is required when until=end_stream")
		}
		return nil
	default:
		return fmt.Errorf("unknown until value %q", until)
	}
}

func validateHeaderList(action scriptTable, key string) error {
	headers, ok, err := action.stringListValue(key)
	if err != nil || !ok {
		return err
	}
	for _, raw := range headers {
		if _, err := parseHeader(raw); err != nil {
			return err
		}
	}
	return nil
}

func validateHexField(action scriptTable, key string) error {
	value, ok, err := action.stringValue(key)
	if err != nil || !ok {
		return err
	}
	_, err = parseHexBytes(value)
	return err
}

func validateExactlyOne(action scriptTable, left, right string) error {
	_, hasLeft := action[left]
	_, hasRight := action[right]
	if hasLeft == hasRight {
		return fmt.Errorf("exactly one of %s or %s must be set", left, right)
	}
	return nil
}

func validateNotBoth(action scriptTable, left, right string) error {
	_, hasLeft := action[left]
	_, hasRight := action[right]
	if hasLeft && hasRight {
		return fmt.Errorf("%s and %s cannot be used together", left, right)
	}
	return nil
}

func parseFlagsOnly(action scriptTable, mapping map[string]uint8) error {
	_, err := parseFlags(action, mapping)
	return err
}

func findScriptActionSpec(name string) (scriptActionSpec, bool) {
	for _, spec := range scriptActionSpecs {
		if spec.name == name {
			return spec, true
		}
	}
	return scriptActionSpec{}, false
}

func scriptKindName(kind scriptValueKind) string {
	switch kind {
	case scriptString:
		return "string"
	case scriptNumber:
		return "integer"
	case scriptBool:
		return "boolean"
	case scriptStringList:
		return "string array"
	default:
		return "unknown"
	}
}

func sortedKeys(src map[string]uint8) []string {
	keys := make([]string, 0, len(src))
	for key := range src {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func flagValue(src map[string]uint8, key string) uint8 {
	return src[key]
}

const requestScriptTemplate = `[connection]
url = "https://nghttp2.org/httpbin/headers"
send_goaway = false
timeout_ms = 5000

[[action]]
type = "preface"

[[action]]
type = "settings"
settings = [
  "ENABLE_PUSH=0",
  "INITIAL_WINDOW_SIZE=65535",
  "HEADER_TABLE_SIZE=8192",
]

[[action]]
type = "receive"
until = "settings"
ack_settings = true

[[action]]
type = "sleep"
duration_ms = 250

[[action]]
type = "headers"
stream_id = 1
flags = ["END_HEADERS", "END_STREAM"]
headers = [
  ":method: GET",
  ":path: /httpbin/headers",
  ":scheme: https",
  ":authority: nghttp2.org",
  "user-agent: mh2c-go-script",
]

[[action]]
type = "receive"
stream_id = 1
until = "end_stream"
ack_ping = true
`
