package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

type scriptFile struct {
	connection scriptTable
	actions    []scriptTable
}

type scriptTable map[string]scriptValue

type scriptValue struct {
	kind    scriptValueKind
	str     string
	number  int64
	boolean bool
	list    []string
}

type scriptValueKind int

const (
	scriptString scriptValueKind = iota + 1
	scriptNumber
	scriptBool
	scriptStringList
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

func applyScriptConnection(cfg config, script scriptFile) (config, error) {
	if value, ok, err := script.connection.stringValue("url"); err != nil {
		return config{}, err
	} else if ok {
		cfg.rawURL = value
	}
	if value, ok, err := script.connection.stringValue("scheme"); err != nil {
		return config{}, err
	} else if ok {
		cfg.scheme = value
	}
	if value, ok, err := script.connection.stringValue("host"); err != nil {
		return config{}, err
	} else if ok {
		cfg.host = value
	}
	if value, ok, err := script.connection.stringValue("authority"); err != nil {
		return config{}, err
	} else if ok {
		cfg.authority = value
	}
	if value, ok, err := script.connection.stringValue("path"); err != nil {
		return config{}, err
	} else if ok {
		cfg.path = value
	}
	if value, ok, err := script.connection.intValue("port"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 || value > 65535 {
			return config{}, fmt.Errorf("connection.port %d is out of range", value)
		}
		cfg.port = uint(value)
	}
	if value, ok, err := script.connection.boolValue("insecure"); err != nil {
		return config{}, err
	} else if ok {
		cfg.insecure = value
	}
	if value, ok, err := script.connection.boolValue("send_goaway"); err != nil {
		return config{}, err
	} else if ok {
		cfg.sendGoAway = value
	}
	if value, ok, err := script.connection.intValue("timeout_ms"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 {
			return config{}, fmt.Errorf("connection.timeout_ms must be >= 0")
		}
		cfg.timeout = time.Duration(value) * time.Millisecond
	}
	if value, ok, err := script.connection.intValue("max_table_size"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 {
			return config{}, fmt.Errorf("connection.max_table_size must be >= 0")
		}
		cfg.maxTable = uint(value)
	}
	return cfg, nil
}

func executeScript(h2c *client.Client, script scriptFile, out io.Writer) (bool, error) {
	sawGoAway := false

	for index, action := range script.actions {
		actionType, err := action.requireString("type")
		if err != nil {
			return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
		}

		switch actionType {
		case "preface":
			if err := h2c.SendConnectionPreface(); err != nil {
				return sawGoAway, err
			}
			fmt.Fprintln(out, ">> CONNECTION_PREFACE")
		case "receive":
			gotGoAway, err := executeReceiveAction(h2c, action, out)
			if err != nil {
				return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
			}
			sawGoAway = sawGoAway || gotGoAway
		default:
			sent, err := buildScriptFrame(h2c, action)
			if err != nil {
				return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
			}
			if err := h2c.SendFrame(sent); err != nil {
				return sawGoAway, err
			}
			applySentFrame(h2c, sent)
			printSentFrame(out, sent)
		}
	}

	return sawGoAway, nil
}

func buildScriptFrame(h2c *client.Client, action scriptTable) (frame.Frame, error) {
	actionType, err := action.requireString("type")
	if err != nil {
		return nil, err
	}

	switch actionType {
	case "settings":
		settingsText, _, err := action.stringListValue("settings")
		if err != nil {
			return nil, err
		}
		settings, err := parseScriptSettings(settingsText)
		if err != nil {
			return nil, err
		}
		flags, err := parseFlags(action, settingsFlagNames)
		if err != nil {
			return nil, err
		}
		return frame.SettingsFrame{Flags: flags, Settings: settings}, nil
	case "headers":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		flags, err := parseFlags(action, headersFlagNames)
		if err != nil {
			return nil, err
		}
		block, err := buildHeaderBlock(h2c, action, "headers", "block_hex")
		if err != nil {
			return nil, err
		}
		frameValue := frame.HeadersFrame{
			StreamID:      streamID,
			Flags:         flags,
			BlockFragment: block,
		}
		if flags&frame.FlagHeadersPriority != 0 {
			priority, err := parsePriority(action)
			if err != nil {
				return nil, err
			}
			frameValue.Priority = priority
		}
		if padLength, ok, err := action.optionalUint8("pad_length"); err != nil {
			return nil, err
		} else if ok {
			frameValue.PadLength = padLength
		}
		return frameValue, nil
	case "continuation":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		flags, err := parseFlags(action, continuationFlagNames)
		if err != nil {
			return nil, err
		}
		blockHex, err := action.requireString("block_hex")
		if err != nil {
			return nil, err
		}
		block, err := parseHexBytes(blockHex)
		if err != nil {
			return nil, err
		}
		return frame.ContinuationFrame{StreamID: streamID, Flags: flags, BlockFragment: block}, nil
	case "data":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		flags, err := parseFlags(action, dataFlagNames)
		if err != nil {
			return nil, err
		}
		data, err := parseDataField(action, "data", "data_hex")
		if err != nil {
			return nil, err
		}
		frameValue := frame.DataFrame{StreamID: streamID, Flags: flags, Data: data}
		if padLength, ok, err := action.optionalUint8("pad_length"); err != nil {
			return nil, err
		} else if ok {
			frameValue.PadLength = padLength
		}
		return frameValue, nil
	case "ping":
		flags, err := parseFlags(action, pingFlagNames)
		if err != nil {
			return nil, err
		}
		payload, err := parsePingActionData(action)
		if err != nil {
			return nil, err
		}
		return frame.PingFrame{Flags: flags, Data: payload}, nil
	case "goaway":
		lastStreamID, err := action.requireUint32("last_stream_id")
		if err != nil {
			return nil, err
		}
		errorCode, err := parseErrorCodeAction(action)
		if err != nil {
			return nil, err
		}
		debugData, err := parseDataField(action, "debug_data", "debug_hex")
		if err != nil {
			return nil, err
		}
		return frame.GoAwayFrame{LastStreamID: lastStreamID, ErrorCode: errorCode, DebugData: debugData}, nil
	case "window_update":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		increment, err := action.requireUint32("increment")
		if err != nil {
			return nil, err
		}
		return frame.WindowUpdateFrame{StreamID: streamID, Increment: increment}, nil
	case "rst_stream":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		errorCode, err := parseErrorCodeAction(action)
		if err != nil {
			return nil, err
		}
		return frame.RSTStreamFrame{StreamID: streamID, ErrorCode: errorCode}, nil
	case "priority":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		priority, err := parsePriority(action)
		if err != nil {
			return nil, err
		}
		return frame.PriorityFrame{
			StreamID:  streamID,
			Exclusive: priority.Exclusive,
			StreamDep: priority.StreamDep,
			Weight:    priority.Weight,
		}, nil
	case "push_promise":
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		promisedStreamID, err := action.requireUint32("promised_stream_id")
		if err != nil {
			return nil, err
		}
		flags, err := parseFlags(action, pushPromiseFlagNames)
		if err != nil {
			return nil, err
		}
		block, err := buildHeaderBlock(h2c, action, "headers", "block_hex")
		if err != nil {
			return nil, err
		}
		frameValue := frame.PushPromiseFrame{
			StreamID:         streamID,
			Flags:            flags,
			PromisedStreamID: promisedStreamID,
			BlockFragment:    block,
		}
		if padLength, ok, err := action.optionalUint8("pad_length"); err != nil {
			return nil, err
		} else if ok {
			frameValue.PadLength = padLength
		}
		return frameValue, nil
	case "raw":
		frameType, err := action.requireUint8("frame_type")
		if err != nil {
			return nil, err
		}
		streamID, err := action.requireUint32("stream_id")
		if err != nil {
			return nil, err
		}
		flags, ok, err := action.intValue("flags")
		if err != nil {
			return nil, err
		}
		if !ok || flags < 0 || flags > 0xff {
			return nil, fmt.Errorf("flags must be set to 0..255 for raw frames")
		}
		payloadHex, err := action.requireString("payload_hex")
		if err != nil {
			return nil, err
		}
		payload, err := parseHexBytes(payloadHex)
		if err != nil {
			return nil, err
		}
		return frame.RawFrameFromParts(frame.Header{
			Type:     frame.Type(frameType),
			Flags:    uint8(flags),
			StreamID: streamID,
		}, payload), nil
	default:
		return nil, fmt.Errorf("unsupported action type %q", actionType)
	}
}

func executeReceiveAction(h2c *client.Client, action scriptTable, out io.Writer) (bool, error) {
	if _, ok := action["count"]; ok {
		if _, ok := action["until"]; ok {
			return false, fmt.Errorf("count and until cannot be used together")
		}
	}

	count := int64(1)
	if value, ok, err := action.intValue("count"); err != nil {
		return false, err
	} else if ok {
		if value <= 0 {
			return false, fmt.Errorf("count must be > 0")
		}
		count = value
	}
	until, _, err := action.stringValue("until")
	if err != nil {
		return false, err
	}
	streamID, hasStreamID, err := action.optionalUint32("stream_id")
	if err != nil {
		return false, err
	}
	ackSettings, _, err := action.boolValue("ack_settings")
	if err != nil {
		return false, err
	}
	ackPing, _, err := action.boolValue("ack_ping")
	if err != nil {
		return false, err
	}
	if until == "end_stream" && !hasStreamID {
		return false, fmt.Errorf("stream_id is required when until=end_stream")
	}

	var (
		receivedCount int64
		sawGoAway     bool
		pendingStream uint32
		pendingBlock  []byte
		pendingEnd    bool
	)

	for {
		received, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		receivedCount++
		printReceivedFrame(out, h2c, received)

		if headers, stream, endStream, err := consumeHeaderBlockForDisplay(&pendingStream, &pendingBlock, &pendingEnd, received, h2c.DecodeHeaders); err != nil {
			return sawGoAway, err
		} else if len(headers) > 0 {
			printHeaderFields(out, headers)
			if until == "end_stream" && hasStreamID && stream == streamID && endStream {
				return sawGoAway, nil
			}
		}

		switch typed := received.(type) {
		case frame.SettingsFrame:
			if ackSettings && typed.Flags&frame.FlagSettingsAck == 0 {
				ack := frame.SettingsFrame{Flags: frame.FlagSettingsAck}
				if err := h2c.SendFrame(ack); err != nil {
					return sawGoAway, err
				}
				printSentFrame(out, ack)
			}
			if until == "settings" && typed.Flags&frame.FlagSettingsAck == 0 {
				return sawGoAway, nil
			}
			if until == "settings_ack" && typed.Flags&frame.FlagSettingsAck != 0 {
				return sawGoAway, nil
			}
		case frame.PingFrame:
			if ackPing && typed.Flags&frame.FlagPingAck == 0 {
				ack := frame.PingFrame{Flags: frame.FlagPingAck, Data: typed.Data}
				if err := h2c.SendFrame(ack); err != nil {
					return sawGoAway, err
				}
				printSentFrame(out, ack)
			}
			if until == "ping_ack" && typed.Flags&frame.FlagPingAck != 0 {
				return sawGoAway, nil
			}
		case frame.GoAwayFrame:
			sawGoAway = true
			if until == "" || until == "goaway" {
				return sawGoAway, nil
			}
		case frame.DataFrame:
			if until == "end_stream" && hasStreamID && typed.StreamID == streamID && typed.Flags&frame.FlagDataEndStream != 0 {
				return sawGoAway, nil
			}
		case frame.HeadersFrame:
			if until == "end_stream" && hasStreamID && typed.StreamID == streamID && typed.Flags&frame.FlagHeadersEndStream != 0 {
				return sawGoAway, nil
			}
		}

		if until == "" && receivedCount >= count {
			return sawGoAway, nil
		}
	}
}

func consumeHeaderBlockForDisplay(
	pendingStream *uint32,
	pendingBlock *[]byte,
	pendingEnd *bool,
	received frame.Frame,
	decode func([]byte) ([]hpack.HeaderField, error),
) ([]hpack.HeaderField, uint32, bool, error) {
	switch typed := received.(type) {
	case frame.HeadersFrame:
		if typed.Flags&frame.FlagHeadersEndHeaders != 0 {
			headers, err := decode(typed.BlockFragment)
			return headers, typed.StreamID, typed.Flags&frame.FlagHeadersEndStream != 0, err
		}
		*pendingStream = typed.StreamID
		*pendingBlock = append([]byte(nil), typed.BlockFragment...)
		*pendingEnd = typed.Flags&frame.FlagHeadersEndStream != 0
	case frame.ContinuationFrame:
		if *pendingStream == 0 {
			return nil, 0, false, fmt.Errorf("unexpected CONTINUATION frame on stream %d", typed.StreamID)
		}
		if typed.StreamID != *pendingStream {
			return nil, 0, false, fmt.Errorf("CONTINUATION stream mismatch: got %d, want %d", typed.StreamID, *pendingStream)
		}
		*pendingBlock = append(*pendingBlock, typed.BlockFragment...)
		if typed.Flags&frame.FlagContinuationEndHeaders != 0 {
			headers, err := decode(*pendingBlock)
			streamID := *pendingStream
			endStream := *pendingEnd
			*pendingStream = 0
			*pendingBlock = nil
			*pendingEnd = false
			return headers, streamID, endStream, err
		}
	}
	return nil, 0, false, nil
}

func applySentFrame(h2c *client.Client, sent frame.Frame) {
	switch typed := sent.(type) {
	case frame.SettingsFrame:
		for _, setting := range typed.Settings {
			if setting.ID == frame.SettingHeaderTableSize {
				h2c.RequestCodec().SetMaxDynamicTableSize(setting.Value)
			}
		}
	}
}

func printSentFrame(out io.Writer, f frame.Frame) {
	fmt.Fprintf(out, ">> %s\n", client.DebugFrameString(f))
	switch typed := f.(type) {
	case frame.SettingsFrame:
		if len(typed.Settings) == 0 {
			fmt.Fprintln(out, "  settings: <empty>")
			return
		}
		for _, setting := range typed.Settings {
			fmt.Fprintf(out, "  setting id=%s value=%d\n", settingName(setting.ID), setting.Value)
		}
	case frame.DataFrame:
		fmt.Fprintf(out, "  data-length: %d\n", len(typed.Data))
		fmt.Fprintf(out, "  data-hex: %s\n", hex.EncodeToString(typed.Data))
		fmt.Fprintf(out, "  data-text: %s\n", formatDataText(typed.Data))
	case frame.PingFrame:
		fmt.Fprintf(out, "  ping-hex: %s\n", hex.EncodeToString(typed.Data[:]))
		fmt.Fprintf(out, "  ping-text: %s\n", formatDataText(typed.Data[:]))
	case frame.GoAwayFrame:
		if len(typed.DebugData) == 0 {
			fmt.Fprintln(out, "  debug-data: <empty>")
			return
		}
		fmt.Fprintf(out, "  debug-data-hex: %s\n", hex.EncodeToString(typed.DebugData))
		fmt.Fprintf(out, "  debug-data-text: %s\n", formatDataText(typed.DebugData))
	default:
		fmt.Fprintf(out, "  payload-hex: %s\n", hex.EncodeToString(f.Payload()))
	}
}

func buildHeaderBlock(h2c *client.Client, action scriptTable, headerKey, hexKey string) ([]byte, error) {
	headers, hasHeaders, err := action.stringListValue(headerKey)
	if err != nil {
		return nil, err
	}
	blockHex, hasHex, err := action.stringValue(hexKey)
	if err != nil {
		return nil, err
	}
	if hasHeaders == hasHex {
		return nil, fmt.Errorf("exactly one of %s or %s must be set", headerKey, hexKey)
	}
	if hasHex {
		return parseHexBytes(blockHex)
	}

	fields := make([]hpack.HeaderField, 0, len(headers))
	for _, raw := range headers {
		field, err := parseHeader(raw)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return h2c.EncodeHeaders(fields)
}

func parseScriptSettings(entries []string) ([]frame.Setting, error) {
	settings := make([]frame.Setting, 0, len(entries))
	for _, entry := range entries {
		key, valueText, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid setting %q", entry)
		}
		settingID, err := parseSettingID(strings.TrimSpace(key))
		if err != nil {
			return nil, err
		}
		value, err := strconv.ParseUint(strings.TrimSpace(valueText), 0, 32)
		if err != nil {
			return nil, err
		}
		settings = append(settings, frame.Setting{ID: settingID, Value: uint32(value)})
	}
	return settings, nil
}

func parsePriority(action scriptTable) (*frame.PriorityParam, error) {
	streamDep, err := action.requireUint32("stream_dep")
	if err != nil {
		return nil, err
	}
	weight, err := action.requireUint8("weight")
	if err != nil {
		return nil, err
	}
	exclusive, _, err := action.boolValue("exclusive")
	if err != nil {
		return nil, err
	}
	return &frame.PriorityParam{Exclusive: exclusive, StreamDep: streamDep, Weight: weight}, nil
}

func parsePingActionData(action scriptTable) ([8]byte, error) {
	text, hasText, err := action.stringValue("data")
	if err != nil {
		return [8]byte{}, err
	}
	hexText, hasHex, err := action.stringValue("data_hex")
	if err != nil {
		return [8]byte{}, err
	}
	if hasText == hasHex {
		return [8]byte{}, fmt.Errorf("exactly one of data or data_hex must be set")
	}
	if hasText {
		return parsePingData(text)
	}
	raw, err := parseHexBytes(hexText)
	if err != nil {
		return [8]byte{}, err
	}
	if len(raw) != 8 {
		return [8]byte{}, fmt.Errorf("data_hex must decode to exactly 8 bytes, got %d", len(raw))
	}
	var payload [8]byte
	copy(payload[:], raw)
	return payload, nil
}

func parseDataField(action scriptTable, textKey, hexKey string) ([]byte, error) {
	text, hasText, err := action.stringValue(textKey)
	if err != nil {
		return nil, err
	}
	hexText, hasHex, err := action.stringValue(hexKey)
	if err != nil {
		return nil, err
	}
	if hasText && hasHex {
		return nil, fmt.Errorf("%s and %s cannot be used together", textKey, hexKey)
	}
	if hasHex {
		return parseHexBytes(hexText)
	}
	return []byte(text), nil
}

func parseHexBytes(src string) ([]byte, error) {
	clean := strings.ReplaceAll(src, " ", "")
	clean = strings.ReplaceAll(clean, "\t", "")
	return hex.DecodeString(clean)
}

func parseFlags(action scriptTable, mapping map[string]uint8) (uint8, error) {
	flagNames, ok, err := action.stringListValue("flags")
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	var flags uint8
	for _, raw := range flagNames {
		name := strings.ToUpper(strings.TrimSpace(raw))
		if value, ok := mapping[name]; ok {
			flags |= value
			continue
		}
		parsed, err := strconv.ParseUint(name, 0, 8)
		if err != nil {
			return 0, fmt.Errorf("unknown flag %q", raw)
		}
		flags |= uint8(parsed)
	}
	return flags, nil
}

func parseSettingID(name string) (frame.SettingID, error) {
	switch strings.ToUpper(name) {
	case "HEADER_TABLE_SIZE", "SETTINGS_HEADER_TABLE_SIZE":
		return frame.SettingHeaderTableSize, nil
	case "ENABLE_PUSH", "SETTINGS_ENABLE_PUSH":
		return frame.SettingEnablePush, nil
	case "MAX_CONCURRENT_STREAMS", "SETTINGS_MAX_CONCURRENT_STREAMS":
		return frame.SettingMaxConcurrentStreams, nil
	case "INITIAL_WINDOW_SIZE", "SETTINGS_INITIAL_WINDOW_SIZE":
		return frame.SettingInitialWindowSize, nil
	case "MAX_FRAME_SIZE", "SETTINGS_MAX_FRAME_SIZE":
		return frame.SettingMaxFrameSize, nil
	case "MAX_HEADER_LIST_SIZE", "SETTINGS_MAX_HEADER_LIST_SIZE":
		return frame.SettingMaxHeaderListSize, nil
	default:
		value, err := strconv.ParseUint(name, 0, 16)
		if err != nil {
			return 0, fmt.Errorf("unknown setting id %q", name)
		}
		return frame.SettingID(value), nil
	}
}

func parseErrorCodeAction(action scriptTable) (frame.ErrorCode, error) {
	if text, ok, err := action.stringValue("error_code"); err != nil {
		return 0, err
	} else if ok {
		switch strings.ToUpper(text) {
		case "NO_ERROR":
			return frame.ErrNo, nil
		default:
			value, err := strconv.ParseUint(text, 0, 32)
			if err != nil {
				return 0, fmt.Errorf("unknown error code %q", text)
			}
			return frame.ErrorCode(value), nil
		}
	}
	return frame.ErrNo, nil
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

func (t scriptTable) requireString(key string) (string, error) {
	value, ok, err := t.stringValue(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func (t scriptTable) stringValue(key string) (string, bool, error) {
	value, ok := t[key]
	if !ok {
		return "", false, nil
	}
	if value.kind != scriptString {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	return value.str, true, nil
}

func (t scriptTable) stringListValue(key string) ([]string, bool, error) {
	value, ok := t[key]
	if !ok {
		return nil, false, nil
	}
	if value.kind != scriptStringList {
		return nil, false, fmt.Errorf("%s must be an array of strings", key)
	}
	return append([]string(nil), value.list...), true, nil
}

func (t scriptTable) boolValue(key string) (bool, bool, error) {
	value, ok := t[key]
	if !ok {
		return false, false, nil
	}
	if value.kind != scriptBool {
		return false, false, fmt.Errorf("%s must be a bool", key)
	}
	return value.boolean, true, nil
}

func (t scriptTable) intValue(key string) (int64, bool, error) {
	value, ok := t[key]
	if !ok {
		return 0, false, nil
	}
	if value.kind != scriptNumber {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	return value.number, true, nil
}

func (t scriptTable) requireUint32(key string) (uint32, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	if value < 0 || value > int64(^uint32(0)) {
		return 0, fmt.Errorf("%s must fit in uint32", key)
	}
	return uint32(value), nil
}

func (t scriptTable) optionalUint32(key string) (uint32, bool, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if value < 0 || value > int64(^uint32(0)) {
		return 0, false, fmt.Errorf("%s must fit in uint32", key)
	}
	return uint32(value), true, nil
}

func (t scriptTable) requireUint8(key string) (uint8, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	if value < 0 || value > 0xff {
		return 0, fmt.Errorf("%s must fit in uint8", key)
	}
	return uint8(value), nil
}

func (t scriptTable) optionalUint8(key string) (uint8, bool, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if value < 0 || value > 0xff {
		return 0, false, fmt.Errorf("%s must fit in uint8", key)
	}
	return uint8(value), true, nil
}

var settingsFlagNames = map[string]uint8{
	"ACK": frame.FlagSettingsAck,
}

var headersFlagNames = map[string]uint8{
	"END_STREAM":  frame.FlagHeadersEndStream,
	"END_HEADERS": frame.FlagHeadersEndHeaders,
	"PADDED":      frame.FlagHeadersPadded,
	"PRIORITY":    frame.FlagHeadersPriority,
}

var continuationFlagNames = map[string]uint8{
	"END_HEADERS": frame.FlagContinuationEndHeaders,
}

var dataFlagNames = map[string]uint8{
	"END_STREAM": frame.FlagDataEndStream,
	"PADDED":     frame.FlagDataPadded,
}

var pingFlagNames = map[string]uint8{
	"ACK": frame.FlagPingAck,
}

var pushPromiseFlagNames = map[string]uint8{
	"END_HEADERS": frame.FlagPushPromiseEndHeaders,
	"PADDED":      frame.FlagPushPromisePadded,
}
