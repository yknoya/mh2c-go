package main

import (
	"encoding/json"
	"fmt"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
	"github.com/yknoya/mh2c-go/internal/framefmt"
)

type jsonHeaderField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type jsonFrameEvent struct {
	Direction      string            `json:"direction"`
	FrameType      string            `json:"frame_type"`
	StreamID       uint32            `json:"stream_id"`
	Flags          uint8             `json:"flags"`
	Summary        string            `json:"summary"`
	PayloadLength  int               `json:"payload_length,omitempty"`
	PayloadHex     string            `json:"payload_hex,omitempty"`
	DataHex        string            `json:"data_hex,omitempty"`
	DataText       string            `json:"data_text,omitempty"`
	DecodedHeaders []jsonHeaderField `json:"decoded_headers,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
	Truncated      bool              `json:"truncated,omitempty"`
}

func (o *outputController) buildJSONEvent(direction string, f frame.Frame, headers []hpack.HeaderField, warnings []string) jsonFrameEvent {
	event := jsonFrameEvent{
		Direction: direction,
		FrameType: frameTypeName(f),
		StreamID:  f.Header().StreamID,
		Flags:     f.Header().Flags,
		Summary:   framefmt.Summary(f),
	}

	if payload, ok := payloadHexForJSON(f, o.showHeaderBlock); ok {
		event.PayloadLength = len(payload)
		event.PayloadHex, event.Truncated = truncateHex(payload, o.dataLimit)
	}

	switch typed := f.(type) {
	case frame.DataFrame:
		o.applyJSONDataPayload(&event, typed.Data)
	case frame.PingFrame:
		o.applyJSONDataPayload(&event, typed.Data[:])
	case frame.GoAwayFrame:
		if len(typed.DebugData) > 0 {
			o.applyJSONDataPayload(&event, typed.DebugData)
		}
	}

	if len(headers) > 0 {
		event.DecodedHeaders = make([]jsonHeaderField, 0, len(headers))
		for _, field := range headers {
			event.DecodedHeaders = append(event.DecodedHeaders, jsonHeaderField{Name: field.Name, Value: field.Value})
		}
	}
	if len(warnings) > 0 {
		event.Warnings = append([]string(nil), warnings...)
	}

	return event
}

func (o *outputController) applyJSONDataPayload(event *jsonFrameEvent, data []byte) {
	event.PayloadLength = len(data)
	switch o.dataFormat {
	case dataFormatText:
		event.DataText, event.Truncated = formatDataTextJSON(data, o.dataLimit)
	case dataFormatHex:
		event.DataHex, event.Truncated = truncateHex(data, o.dataLimit)
	default:
		event.DataHex, event.Truncated = truncateHex(data, o.dataLimit)
		event.DataText, _ = formatDataTextJSON(data, o.dataLimit)
	}
}

func (o *outputController) writeJSON(event jsonFrameEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(o.out, "%s\n", data)
	return err
}

func payloadHexForJSON(f frame.Frame, showHeaderBlock bool) ([]byte, bool) {
	switch typed := f.(type) {
	case frame.HeadersFrame:
		if !showHeaderBlock {
			return nil, false
		}
		return typed.BlockFragment, true
	case frame.ContinuationFrame:
		if !showHeaderBlock {
			return nil, false
		}
		return typed.BlockFragment, true
	case frame.PushPromiseFrame:
		if !showHeaderBlock {
			return nil, false
		}
		return typed.BlockFragment, true
	case frame.DataFrame, frame.PingFrame, frame.GoAwayFrame:
		return nil, false
	default:
		return f.Payload(), true
	}
}
