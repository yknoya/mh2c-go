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
	if err := controller.HandleSent(h2c, frame.DataFrame{StreamID: 1, Data: []byte("sent")}); err != nil {
		t.Fatalf("HandleSent(data stream=1) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.SettingsFrame{}); err != nil {
		t.Fatalf("HandleReceived(settings) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.DataFrame{StreamID: 3, Data: []byte("skip")}); err != nil {
		t.Fatalf("HandleReceived(data stream=3) error = %v", err)
	}
	if err := controller.HandleReceived(h2c, frame.DataFrame{StreamID: 1, Data: []byte("keep")}); err != nil {
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
	if err := controller.HandleReceived(h2c, frame.DataFrame{
		StreamID: 3,
		Flags:    frame.FlagDataEndStream,
		Data:     []byte("hello"),
	}); err != nil {
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
