package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestOutputControllerAppliesFrameStreamAndDirectionFilters(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:             "observe",
		outputFormat:     outputFormatText,
		dataFormat:       dataFormatBoth,
		decodeHeaders:    true,
		showHeaderBlock:  true,
		frameFilters:     stringFlags{"data"},
		directionFilters: stringFlags{"received"},
		hasStreamFilter:  true,
		streamFilter:     1,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	if err := controller.HandleSent(h2c, frame.NewDataFrame(1, 0, []byte("sent"))); err != nil {
		t.Fatalf("HandleSent(data stream=1) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.SettingsFrame{}); err != nil {
		t.Fatalf("HandleReceived(settings) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.NewDataFrame(3, 0, []byte("skip"))); err != nil {
		t.Fatalf("HandleReceived(data stream=3) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.NewDataFrame(1, 0, []byte("keep"))); err != nil {
		t.Fatalf("HandleReceived(data stream=1) error = %v", err)
	}

	text := out.String()
	if strings.Contains(text, "SETTINGS") || strings.Contains(text, "stream=3") || strings.Contains(text, "sent") {
		t.Fatalf("unexpected filtered output: %q", text)
	}
	if !strings.Contains(text, "DATA stream=1") {
		t.Fatalf("output = %q, want DATA stream=1", text)
	}
}

func TestOutputControllerDisplaysDecodedSentHeaders(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "request",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	fields := []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/demo"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
	}
	block, err := h2c.EncodeHeaders(fields)
	if err != nil {
		t.Fatalf("EncodeHeaders() error = %v", err)
	}
	if err := controller.HandleSent(h2c, frame.HeadersFrame{
		StreamID:      1,
		Flags:         frame.FlagHeadersEndHeaders | frame.FlagHeadersEndStream,
		BlockFragment: block,
	}); err != nil {
		t.Fatalf("HandleSent(headers) error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "header-block-fragment:") {
		t.Fatalf("output = %q, want header block fragment", text)
	}
	if !strings.Contains(text, "header :method: GET") || !strings.Contains(text, "header :authority: example.com") {
		t.Fatalf("output = %q, want decoded sent headers", text)
	}
}

func TestOutputControllerJSONLIncludesDecodedHeaders(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "observe",
		outputFormat:    outputFormatJSONL,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	block, err := h2c.ResponseCodec().Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.HeadersFrame{
		StreamID:      1,
		Flags:         frame.FlagHeadersEndHeaders | frame.FlagHeadersEndStream,
		BlockFragment: block,
	}); err != nil {
		t.Fatalf("HandleReceived() error = %v", err)
	}

	var event jsonFrameEvent
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.FrameType != "headers" || len(event.DecodedHeaders) == 0 {
		t.Fatalf("event = %#v", event)
	}
	if event.DecodedHeaders[0].Name != ":status" || event.DecodedHeaders[0].Value != "200" {
		t.Fatalf("decoded headers = %#v", event.DecodedHeaders)
	}
}

func TestOutputControllerJSONLMarksTruncatedDataTextSafely(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:         "observe",
		outputFormat: outputFormatJSONL,
		dataFormat:   dataFormatText,
		dataLimit:    5,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	if err := controller.HandleReceived(h2c, frame.NewDataFrame(1, 0, []byte("あい"))); err != nil {
		t.Fatalf("HandleReceived(data) error = %v", err)
	}

	event := decodeJSONFrameEvent(t, out.Bytes())
	if event.FrameType != "data" || event.PayloadLength != len([]byte("あい")) {
		t.Fatalf("event = %#v", event)
	}
	if event.DataText != "\"あ\" (truncated)" {
		t.Fatalf("DataText = %q, want truncated UTF-8-safe prefix", event.DataText)
	}
	if !event.Truncated {
		t.Fatalf("event = %#v, want truncated=true", event)
	}
}

func TestOutputControllerJSONLMarksTruncatedHeaderBlock(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "observe",
		outputFormat:    outputFormatJSONL,
		dataFormat:      dataFormatBoth,
		dataLimit:       3,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	block, err := h2c.ResponseCodec().Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.HeadersFrame{
		StreamID:      1,
		Flags:         frame.FlagHeadersEndHeaders,
		BlockFragment: block,
	}); err != nil {
		t.Fatalf("HandleReceived(headers) error = %v", err)
	}

	event := decodeJSONFrameEvent(t, out.Bytes())
	wantPayloadHex, wantTruncated := truncateHex(block, 3)
	if event.FrameType != "headers" || event.PayloadLength != len(block) {
		t.Fatalf("event = %#v", event)
	}
	if event.PayloadHex != wantPayloadHex {
		t.Fatalf("PayloadHex = %q, want %q", event.PayloadHex, wantPayloadHex)
	}
	if event.Truncated != wantTruncated || !event.Truncated {
		t.Fatalf("event = %#v, want truncated header block", event)
	}
}

func TestOutputControllerJSONLRejectsInvalidHPACKBlock(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "observe",
		outputFormat:    outputFormatJSONL,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	err = controller.HandleReceived(h2c, frame.HeadersFrame{
		StreamID:      1,
		Flags:         frame.FlagHeadersEndHeaders,
		BlockFragment: buildInvalidHeaderBlock(t),
	})
	if err == nil {
		t.Fatal("HandleReceived(headers) error = nil, want HPACK decode error")
	}
	if !strings.Contains(err.Error(), "dynamic table size update too large") {
		t.Fatalf("HandleReceived(headers) error = %v, want dynamic table size update error", err)
	}
}

func TestOutputControllerFlushesAutoCapturedResponse(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	bodyPath := filepath.Join(tmpDir, "body.bin")
	headersPath := filepath.Join(tmpDir, "headers.txt")

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "observe",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
		saveBody:        bodyPath,
		saveHeaders:     headersPath,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	block, err := h2c.ResponseCodec().Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.HeadersFrame{
		StreamID:      3,
		Flags:         frame.FlagHeadersEndHeaders,
		BlockFragment: block,
	}); err != nil {
		t.Fatalf("HandleReceived(headers) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.NewDataFrame(3, frame.FlagDataEndStream, []byte("hello"))); err != nil {
		t.Fatalf("HandleReceived(data) error = %v", err)
	}
	if err := controller.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatalf("ReadFile(body) error = %v", err)
	}
	headers, err := os.ReadFile(headersPath)
	if err != nil {
		t.Fatalf("ReadFile(headers) error = %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
	if !strings.Contains(string(headers), ":status: 200") {
		t.Fatalf("headers = %q, want :status: 200", headers)
	}
}

func TestOutputControllerFiltersPushPromiseAndRawFrames(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "observe",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatHex,
		decodeHeaders:   false,
		showHeaderBlock: true,
		frameFilters:    stringFlags{"push_promise", "raw"},
		hasStreamFilter: true,
		streamFilter:    1,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	if err := controller.HandleReceived(h2c, frame.PushPromiseFrame{
		StreamID:         1,
		Flags:            frame.FlagPushPromiseEndHeaders,
		PromisedStreamID: 2,
		BlockFragment:    []byte{0x82},
	}); err != nil {
		t.Fatalf("HandleReceived(push_promise) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.RawFrameFromParts(frame.Header{
		Type:     frame.Type(0xfe),
		Flags:    0x3,
		StreamID: 1,
	}, []byte{0xde, 0xad, 0xbe, 0xef})); err != nil {
		t.Fatalf("HandleReceived(raw) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.ContinuationFrame{
		StreamID:      1,
		Flags:         frame.FlagContinuationEndHeaders,
		BlockFragment: []byte{0x80},
	}); err != nil {
		t.Fatalf("HandleReceived(continuation) error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "PUSH_PROMISE stream=1 len=5 type=PUSH_PROMISE(0x05) flags=0x04") ||
		!strings.Contains(text, "promised=2") {
		t.Fatalf("output = %q, want PUSH_PROMISE", text)
	}
	if !strings.Contains(text, "promised-stream-id: 2") {
		t.Fatalf("output = %q, want promised stream detail", text)
	}
	if !strings.Contains(text, "raw-payload-hex: deadbeef") {
		t.Fatalf("output = %q, want raw payload", text)
	}
	if strings.Contains(text, "CONTINUATION") {
		t.Fatalf("output = %q, did not expect CONTINUATION", text)
	}
}

func decodeJSONFrameEvent(t *testing.T, data []byte) jsonFrameEvent {
	t.Helper()

	var event jsonFrameEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return event
}

func buildInvalidHeaderBlock(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	enc.SetMaxDynamicTableSizeLimit(8192)
	enc.SetMaxDynamicTableSize(8192)
	if err := enc.WriteField(hpack.HeaderField{Name: "x-first", Value: "one"}); err != nil {
		t.Fatalf("WriteField(x-first) error = %v", err)
	}
	enc.SetMaxDynamicTableSize(2048)
	if err := enc.WriteField(hpack.HeaderField{Name: "x-second", Value: "two"}); err != nil {
		t.Fatalf("WriteField(x-second) error = %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}
