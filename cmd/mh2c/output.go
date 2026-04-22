package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

const (
	outputFormatText  = "text"
	outputFormatJSONL = "jsonl"

	dataFormatText = "text"
	dataFormatHex  = "hex"
	dataFormatBoth = "both"
)

type outputController struct {
	out             io.Writer
	format          string
	dataFormat      string
	dataLimit       uint
	decodeHeaders   bool
	showHeaderBlock bool
	frameFilters    map[string]bool
	hasStreamFilter bool
	streamFilter    uint32
	pendingStream   uint32
	pendingBlock    []byte
	pendingEnd      bool
	capture         *captureManager
}

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

type captureManager struct {
	bodyPath       string
	headersPath    string
	explicitStream bool
	targetStreamID uint32
	selectedStream uint32
	streams        map[uint32]*capturedStream
}

type capturedStream struct {
	headers []hpack.HeaderField
	body    bytes.Buffer
	done    bool
}

func newOutputController(out io.Writer, cfg config) (*outputController, error) {
	filters, err := buildFrameFilterSet(cfg.frameFilters)
	if err != nil {
		return nil, err
	}

	controller := &outputController{
		out:             out,
		format:          cfg.outputFormat,
		dataFormat:      cfg.dataFormat,
		dataLimit:       cfg.dataLimit,
		decodeHeaders:   cfg.decodeHeaders,
		showHeaderBlock: cfg.showHeaderBlock,
		frameFilters:    filters,
		hasStreamFilter: cfg.hasStreamFilter,
		streamFilter:    uint32(cfg.streamFilter),
	}

	switch cfg.mode {
	case "request":
		if cfg.saveBody != "" || cfg.saveHeaders != "" {
			targetStream := uint32(cfg.streamID)
			if cfg.hasStreamFilter {
				targetStream = uint32(cfg.streamFilter)
			}
			controller.capture = newExplicitCaptureManager(targetStream, cfg.saveBody, cfg.saveHeaders)
		}
	case "observe":
		if cfg.saveBody != "" || cfg.saveHeaders != "" {
			if cfg.hasStreamFilter {
				controller.capture = newExplicitCaptureManager(uint32(cfg.streamFilter), cfg.saveBody, cfg.saveHeaders)
			} else {
				controller.capture = newAutoCaptureManager(cfg.saveBody, cfg.saveHeaders)
			}
		}
	}

	return controller, nil
}

func newExplicitCaptureManager(streamID uint32, bodyPath, headersPath string) *captureManager {
	return &captureManager{
		bodyPath:       bodyPath,
		headersPath:    headersPath,
		explicitStream: true,
		targetStreamID: streamID,
		streams:        map[uint32]*capturedStream{},
	}
}

func newAutoCaptureManager(bodyPath, headersPath string) *captureManager {
	return &captureManager{
		bodyPath:    bodyPath,
		headersPath: headersPath,
		streams:     map[uint32]*capturedStream{},
	}
}

func (o *outputController) PrintNotice(direction, kind, summary string) error {
	if o.format == outputFormatJSONL {
		return o.writeJSON(jsonFrameEvent{
			Direction: direction,
			FrameType: kind,
			Summary:   summary,
		})
	}
	prefix := "<<"
	if direction == "sent" {
		prefix = ">>"
	}
	_, err := fmt.Fprintf(o.out, "%s %s\n", prefix, summary)
	return err
}

func (o *outputController) HandleSent(f frame.Frame) error {
	if !o.shouldDisplay(f) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("sent", f, nil, nil))
	}
	return o.writeTextFrame(">>", f, nil, nil)
}

func (o *outputController) HandleReceived(h2c *client.Client, f frame.Frame) error {
	headers, warnings, streamID, endStream, err := o.decodeReceivedHeaders(h2c, f)
	if err != nil {
		return err
	}
	o.captureReceived(streamID, headers, endStream, f)
	if !o.shouldDisplay(f) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("received", f, headers, warnings))
	}
	return o.writeTextFrame("<<", f, headers, warnings)
}

func (o *outputController) Flush() error {
	if o.capture == nil {
		return nil
	}
	return o.capture.Flush()
}

func (o *outputController) decodeReceivedHeaders(h2c *client.Client, f frame.Frame) ([]hpack.HeaderField, []string, uint32, bool, error) {
	needTrackedDecode := o.decodeHeaders || o.capture != nil
	if needTrackedDecode {
		headers, warnings, streamID, endStream, err := consumeHeaderBlockForDisplay(
			&o.pendingStream,
			&o.pendingBlock,
			&o.pendingEnd,
			f,
			h2c.DecodeHeadersDetailed,
		)
		if err != nil {
			return nil, nil, 0, false, err
		}
		if len(headers) > 0 {
			return headers, warnings, streamID, endStream, nil
		}
	}

	if !o.decodeHeaders {
		return nil, nil, 0, false, nil
	}

	typed, ok := f.(frame.PushPromiseFrame)
	if !ok || typed.Flags&frame.FlagPushPromiseEndHeaders == 0 {
		return nil, nil, 0, false, nil
	}
	report, err := h2c.DecodeHeadersDetailed(typed.BlockFragment)
	if err != nil {
		return nil, nil, 0, false, err
	}
	return report.Fields, report.Warnings, typed.StreamID, false, nil
}

func (o *outputController) captureReceived(streamID uint32, headers []hpack.HeaderField, endStream bool, f frame.Frame) {
	if o.capture == nil {
		return
	}

	switch typed := f.(type) {
	case frame.HeadersFrame:
		if len(headers) > 0 {
			o.capture.RecordHeaders(streamID, headers, endStream)
		}
	case frame.ContinuationFrame:
		if len(headers) > 0 {
			o.capture.RecordHeaders(streamID, headers, endStream)
		}
	case frame.DataFrame:
		o.capture.RecordData(typed.StreamID, typed.Data, typed.Flags&frame.FlagDataEndStream != 0)
	}
}

func (o *outputController) shouldDisplay(f frame.Frame) bool {
	if len(o.frameFilters) > 0 && !o.frameFilters[frameTypeName(f)] {
		return false
	}
	if o.hasStreamFilter {
		streamID := f.Header().StreamID
		if streamID != 0 && streamID != o.streamFilter {
			return false
		}
	}
	return true
}

func (o *outputController) writeTextFrame(prefix string, f frame.Frame, headers []hpack.HeaderField, warnings []string) error {
	if _, err := fmt.Fprintf(o.out, "%s %s\n", prefix, client.DebugFrameString(f)); err != nil {
		return err
	}

	switch typed := f.(type) {
	case frame.SettingsFrame:
		if len(typed.Settings) == 0 {
			if _, err := fmt.Fprintln(o.out, "  settings: <empty>"); err != nil {
				return err
			}
		} else {
			for _, setting := range typed.Settings {
				if _, err := fmt.Fprintf(o.out, "  setting id=%s value=%d\n", settingName(setting.ID), setting.Value); err != nil {
					return err
				}
			}
		}
	case frame.HeadersFrame:
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  header-block-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.ContinuationFrame:
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  continuation-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.PushPromiseFrame:
		if _, err := fmt.Fprintf(o.out, "  promised-stream-id: %d\n", typed.PromisedStreamID); err != nil {
			return err
		}
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  header-block-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.DataFrame:
		if _, err := fmt.Fprintf(o.out, "  data-length: %d\n", len(typed.Data)); err != nil {
			return err
		}
		if err := o.writeTextPayload("data", typed.Data); err != nil {
			return err
		}
	case frame.PingFrame:
		if err := o.writeTextPayload("ping", typed.Data[:]); err != nil {
			return err
		}
	case frame.GoAwayFrame:
		if len(typed.DebugData) == 0 {
			if _, err := fmt.Fprintln(o.out, "  debug-data: <empty>"); err != nil {
				return err
			}
		} else if err := o.writeTextPayload("debug-data", typed.DebugData); err != nil {
			return err
		}
	case frame.RawFrame:
		if _, err := fmt.Fprintf(o.out, "  raw-payload-hex: %s\n", formatHexSummary(typed.Payload(), o.dataLimit)); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(o.out, "  payload-hex: %s\n", formatHexSummary(f.Payload(), o.dataLimit)); err != nil {
			return err
		}
	}

	for _, warning := range warnings {
		if _, err := fmt.Fprintf(o.out, "  header-warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, field := range headers {
		if _, err := fmt.Fprintf(o.out, "  header %s: %s\n", field.Name, field.Value); err != nil {
			return err
		}
	}
	return nil
}

func (o *outputController) writeTextPayload(label string, data []byte) error {
	switch o.dataFormat {
	case dataFormatText:
		_, err := fmt.Fprintf(o.out, "  %s-text: %s\n", label, formatDataTextLimited(data, o.dataLimit))
		return err
	case dataFormatHex:
		_, err := fmt.Fprintf(o.out, "  %s-hex: %s\n", label, formatHexSummary(data, o.dataLimit))
		return err
	default:
		if _, err := fmt.Fprintf(o.out, "  %s-hex: %s\n", label, formatHexSummary(data, o.dataLimit)); err != nil {
			return err
		}
		_, err := fmt.Fprintf(o.out, "  %s-text: %s\n", label, formatDataTextLimited(data, o.dataLimit))
		return err
	}
}

func (o *outputController) buildJSONEvent(direction string, f frame.Frame, headers []hpack.HeaderField, warnings []string) jsonFrameEvent {
	event := jsonFrameEvent{
		Direction: direction,
		FrameType: frameTypeName(f),
		StreamID:  f.Header().StreamID,
		Flags:     f.Header().Flags,
		Summary:   client.DebugFrameString(f),
	}

	if payload, ok := payloadHexForJSON(f, o.showHeaderBlock); ok {
		event.PayloadLength = len(payload)
		event.PayloadHex, event.Truncated = truncateHex(payload, o.dataLimit)
	}

	switch typed := f.(type) {
	case frame.DataFrame:
		event.PayloadLength = len(typed.Data)
		switch o.dataFormat {
		case dataFormatText:
			event.DataText, event.Truncated = formatDataTextJSON(typed.Data, o.dataLimit)
		case dataFormatHex:
			event.DataHex, event.Truncated = truncateHex(typed.Data, o.dataLimit)
		default:
			event.DataHex, event.Truncated = truncateHex(typed.Data, o.dataLimit)
			event.DataText, _ = formatDataTextJSON(typed.Data, o.dataLimit)
		}
	case frame.PingFrame:
		event.PayloadLength = len(typed.Data)
		switch o.dataFormat {
		case dataFormatText:
			event.DataText, event.Truncated = formatDataTextJSON(typed.Data[:], o.dataLimit)
		case dataFormatHex:
			event.DataHex, event.Truncated = truncateHex(typed.Data[:], o.dataLimit)
		default:
			event.DataHex, event.Truncated = truncateHex(typed.Data[:], o.dataLimit)
			event.DataText, _ = formatDataTextJSON(typed.Data[:], o.dataLimit)
		}
	case frame.GoAwayFrame:
		if len(typed.DebugData) > 0 {
			event.PayloadLength = len(typed.DebugData)
			switch o.dataFormat {
			case dataFormatText:
				event.DataText, event.Truncated = formatDataTextJSON(typed.DebugData, o.dataLimit)
			case dataFormatHex:
				event.DataHex, event.Truncated = truncateHex(typed.DebugData, o.dataLimit)
			default:
				event.DataHex, event.Truncated = truncateHex(typed.DebugData, o.dataLimit)
				event.DataText, _ = formatDataTextJSON(typed.DebugData, o.dataLimit)
			}
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

func (o *outputController) writeJSON(event jsonFrameEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(o.out, "%s\n", data)
	return err
}

func (m *captureManager) RecordHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) {
	stream := m.stream(streamID)
	stream.headers = append(stream.headers, headers...)
	if endStream {
		stream.done = true
		m.selectStream(streamID)
	}
}

func (m *captureManager) RecordData(streamID uint32, data []byte, endStream bool) {
	stream := m.stream(streamID)
	_, _ = stream.body.Write(data)
	if endStream {
		stream.done = true
		m.selectStream(streamID)
	}
}

func (m *captureManager) Flush() error {
	stream, ok := m.selected()
	if !ok {
		return nil
	}
	if m.bodyPath != "" {
		if err := os.WriteFile(m.bodyPath, stream.body.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if m.headersPath != "" {
		var body strings.Builder
		for _, field := range stream.headers {
			if _, err := fmt.Fprintf(&body, "%s: %s\n", field.Name, field.Value); err != nil {
				return err
			}
		}
		if err := os.WriteFile(m.headersPath, []byte(body.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *captureManager) stream(streamID uint32) *capturedStream {
	stream, ok := m.streams[streamID]
	if !ok {
		stream = &capturedStream{}
		m.streams[streamID] = stream
	}
	return stream
}

func (m *captureManager) selectStream(streamID uint32) {
	if m.explicitStream {
		if streamID == m.targetStreamID {
			m.selectedStream = streamID
		}
		return
	}
	if m.selectedStream == 0 {
		m.selectedStream = streamID
	}
}

func (m *captureManager) selected() (*capturedStream, bool) {
	if m.explicitStream {
		stream, ok := m.streams[m.targetStreamID]
		return stream, ok
	}
	if m.selectedStream == 0 {
		return nil, false
	}
	stream, ok := m.streams[m.selectedStream]
	return stream, ok
}

func buildFrameFilterSet(values []string) (map[string]bool, error) {
	if len(values) == 0 {
		return nil, nil
	}
	filters := make(map[string]bool, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if !isSupportedFrameFilter(name) {
			return nil, fmt.Errorf("invalid frame-filter %q", value)
		}
		filters[name] = true
	}
	return filters, nil
}

func isSupportedFrameFilter(name string) bool {
	switch name {
	case "data", "headers", "priority", "rst_stream", "settings", "push_promise", "ping", "goaway", "window_update", "continuation", "raw":
		return true
	default:
		return false
	}
}

func frameTypeName(f frame.Frame) string {
	switch f.(type) {
	case frame.DataFrame:
		return "data"
	case frame.HeadersFrame:
		return "headers"
	case frame.PriorityFrame:
		return "priority"
	case frame.RSTStreamFrame:
		return "rst_stream"
	case frame.SettingsFrame:
		return "settings"
	case frame.PushPromiseFrame:
		return "push_promise"
	case frame.PingFrame:
		return "ping"
	case frame.GoAwayFrame:
		return "goaway"
	case frame.WindowUpdateFrame:
		return "window_update"
	case frame.ContinuationFrame:
		return "continuation"
	case frame.RawFrame:
		return "raw"
	default:
		return strings.ToLower(strconv.FormatUint(uint64(f.Header().Type), 10))
	}
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

func truncateHex(data []byte, limit uint) (string, bool) {
	if limit == 0 || len(data) <= int(limit) {
		return hex.EncodeToString(data), false
	}
	return fmt.Sprintf("%s...(truncated %d/%d bytes)", hex.EncodeToString(data[:limit]), limit, len(data)), true
}

func formatHexSummary(data []byte, limit uint) string {
	text, _ := truncateHex(data, limit)
	return text
}

func formatDataTextLimited(data []byte, limit uint) string {
	text, _ := formatDataTextJSON(data, limit)
	return text
}

func formatDataTextJSON(data []byte, limit uint) (string, bool) {
	if len(data) == 0 {
		return "<empty>", false
	}

	truncated := false
	if limit > 0 && len(data) > int(limit) {
		data = truncateUTF8Prefix(data[:limit])
		truncated = true
	}

	if utf8.Valid(data) {
		text := strconv.Quote(string(data))
		if truncated {
			text += " (truncated)"
		}
		return text, truncated
	}
	return "<non-utf8>", truncated
}

func truncateUTF8Prefix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for len(data) > 0 {
		data = data[:len(data)-1]
		if utf8.Valid(data) {
			return data
		}
	}
	return nil
}
