package main

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

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
