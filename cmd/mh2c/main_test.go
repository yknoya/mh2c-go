package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestResolveEndpointWithURL(t *testing.T) {
	t.Parallel()

	got, err := resolveEndpoint(config{
		rawURL: "https://example.com:8443/demo?q=1",
	})
	if err != nil {
		t.Fatalf("resolveEndpoint() error = %v", err)
	}
	if got.scheme != "https" || got.host != "example.com" || got.authority != "example.com:8443" || got.port != 8443 || got.path != "/demo?q=1" {
		t.Fatalf("resolveEndpoint() = %#v", got)
	}
}

func TestParseHeaderPseudoHeader(t *testing.T) {
	t.Parallel()

	got, err := parseHeader(":authority: example.com:8443")
	if err != nil {
		t.Fatalf("parseHeader() error = %v", err)
	}
	if got.Name != ":authority" || got.Value != "example.com:8443" {
		t.Fatalf("parseHeader() = %#v", got)
	}
}

func TestBuildRequestFieldsAddsContentLength(t *testing.T) {
	t.Parallel()

	fields, err := buildRequestFields(endpoint{
		scheme:    "https",
		authority: "example.com",
		path:      "/demo",
	}, config{
		method: "POST",
		headers: headerFlags{
			"user-agent: mh2c-go-test",
		},
	}, []byte("hello"))
	if err != nil {
		t.Fatalf("buildRequestFields() error = %v", err)
	}
	if fieldValue(fields, ":method") != "POST" {
		t.Fatalf(":method = %q, want POST", fieldValue(fields, ":method"))
	}
	if fieldValue(fields, "content-length") != "5" {
		t.Fatalf("content-length = %q, want 5", fieldValue(fields, "content-length"))
	}
	if fieldValue(fields, "user-agent") != "mh2c-go-test" {
		t.Fatalf("user-agent = %q, want mh2c-go-test", fieldValue(fields, "user-agent"))
	}
}

func TestResponseStateConsumesContinuation(t *testing.T) {
	t.Parallel()

	codec := hpack.NewCodec(4096)
	block, err := codec.Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	split := len(block) / 2
	state := responseState{streamID: 1}

	result, err := state.Consume(frame.HeadersFrame{
		StreamID:      1,
		BlockFragment: append([]byte(nil), block[:split]...),
	}, codec.DecodeDetailed)
	if err != nil {
		t.Fatalf("Consume(HEADERS) error = %v", err)
	}
	if len(result.headers) != 0 || result.done {
		t.Fatalf("Consume(HEADERS) = %#v, want pending state", result)
	}

	result, err = state.Consume(frame.ContinuationFrame{
		StreamID:      1,
		Flags:         frame.FlagContinuationEndHeaders,
		BlockFragment: append([]byte(nil), block[split:]...),
	}, codec.DecodeDetailed)
	if err != nil {
		t.Fatalf("Consume(CONTINUATION) error = %v", err)
	}
	if fieldValue(result.headers, ":status") != "200" {
		t.Fatalf(":status = %q, want 200", fieldValue(result.headers, ":status"))
	}

	result, err = state.Consume(frame.DataFrame{
		StreamID: 1,
		Flags:    frame.FlagDataEndStream,
		Data:     []byte("hello"),
	}, codec.DecodeDetailed)
	if err != nil {
		t.Fatalf("Consume(DATA) error = %v", err)
	}
	if !result.done || !bytes.Equal(result.data, []byte("hello")) {
		t.Fatalf("Consume(DATA) = %#v, want done with hello", result)
	}
}

func TestParseConfigObserveMode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg, err := parseConfig([]string{
		"--mode", "observe",
		"--output", "jsonl",
		"--data-format", "hex",
		"--stream-filter", "3",
		"--frame-filter", "data",
		"--max-recv", "10",
	}, &stderr)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.mode != "observe" || cfg.outputFormat != outputFormatJSONL || cfg.dataFormat != dataFormatHex {
		t.Fatalf("parseConfig() = %#v", cfg)
	}
	if !cfg.hasStreamFilter || cfg.streamFilter != 3 || cfg.maxRecv != 10 {
		t.Fatalf("stream filter/max recv = %#v", cfg)
	}
	if len(cfg.frameFilters) != 1 || cfg.frameFilters[0] != "data" {
		t.Fatalf("frameFilters = %#v", cfg.frameFilters)
	}
}

func TestParseConfigRejectsSaveBodyOutsideRequestOrObserve(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	_, err := parseConfig([]string{
		"--mode", "ping",
		"--save-body", "body.bin",
	}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "save-body and save-headers are only supported") {
		t.Fatalf("parseConfig() error = %v, want save-body validation", err)
	}
}

func fieldValue(fields []hpack.HeaderField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}
