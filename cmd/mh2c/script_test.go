package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestParseScript(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[connection]
url = "https://example.com:8443/demo"
insecure = true
timeout_ms = 1500

[[action]]
type = "preface"

[[action]]
type = "settings"
settings = [
  "ENABLE_PUSH=0",
  "INITIAL_WINDOW_SIZE=65535",
]

[[action]]
type = "receive"
until = "settings"
ack_settings = true
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	if got, _, _ := script.connection.stringValue("url"); got != "https://example.com:8443/demo" {
		t.Fatalf("connection.url = %q", got)
	}
	if len(script.actions) != 3 {
		t.Fatalf("len(actions) = %d, want 3", len(script.actions))
	}
}

func TestParseScriptWithSleepAction(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "sleep"
duration_ms = 250
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	if len(script.actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(script.actions))
	}
	if got, ok, err := script.actions[0].intValue("duration_ms"); err != nil || !ok || got != 250 {
		t.Fatalf("duration_ms = %d, ok = %t, err = %v", got, ok, err)
	}
}

func TestApplyScriptConnection(t *testing.T) {
	t.Parallel()

	cfg, err := applyScriptConnection(config{}, scriptFile{
		connection: scriptTable{
			"url":            {kind: scriptString, str: "https://example.com/path"},
			"max_table_size": {kind: scriptNumber, number: 16384},
			"send_goaway":    {kind: scriptBool, boolean: false},
		},
	})
	if err != nil {
		t.Fatalf("applyScriptConnection() error = %v", err)
	}
	if cfg.rawURL != "https://example.com/path" || cfg.maxTable != 16384 || cfg.sendGoAway {
		t.Fatalf("applyScriptConnection() = %#v", cfg)
	}
}

func TestBuildScriptFrameHeaders(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	got, err := buildScriptFrame(h2c, scriptTable{
		"type":      {kind: scriptString, str: "headers"},
		"stream_id": {kind: scriptNumber, number: 1},
		"flags":     {kind: scriptStringList, list: []string{"END_HEADERS", "END_STREAM"}},
		"headers":   {kind: scriptStringList, list: []string{":method: GET", ":path: /"}},
	})
	if err != nil {
		t.Fatalf("buildScriptFrame() error = %v", err)
	}
	typed, ok := got.(frame.HeadersFrame)
	if !ok {
		t.Fatalf("frame type = %T, want HeadersFrame", got)
	}
	if typed.StreamID != 1 || typed.Flags != frame.FlagHeadersEndHeaders|frame.FlagHeadersEndStream || len(typed.BlockFragment) == 0 {
		t.Fatalf("HeadersFrame = %#v", typed)
	}
}

func TestConsumeHeaderBlockForDisplay(t *testing.T) {
	t.Parallel()

	codec := hpack.NewCodec(4096)
	block, err := codec.Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var (
		pendingStream uint32
		pendingBlock  []byte
		pendingEnd    bool
	)
	headers, warnings, streamID, endStream, err := consumeHeaderBlockForDisplay(&pendingStream, &pendingBlock, &pendingEnd, frame.HeadersFrame{
		StreamID:      1,
		BlockFragment: block[:len(block)/2],
	}, codec.DecodeDetailed)
	if err != nil {
		t.Fatalf("consumeHeaderBlockForDisplay(HEADERS) error = %v", err)
	}
	if len(headers) != 0 || len(warnings) != 0 || streamID != 0 || endStream {
		t.Fatalf("HEADERS result = %#v, %#v, %d, %t", headers, warnings, streamID, endStream)
	}
	headers, warnings, streamID, endStream, err = consumeHeaderBlockForDisplay(&pendingStream, &pendingBlock, &pendingEnd, frame.ContinuationFrame{
		StreamID:      1,
		Flags:         frame.FlagContinuationEndHeaders,
		BlockFragment: block[len(block)/2:],
	}, codec.DecodeDetailed)
	if err != nil {
		t.Fatalf("consumeHeaderBlockForDisplay(CONTINUATION) error = %v", err)
	}
	if streamID != 1 || len(warnings) != 0 || endStream || fieldValue(headers, ":status") != "200" {
		t.Fatalf("headers = %#v, warnings = %#v, streamID = %d, endStream = %t", headers, warnings, streamID, endStream)
	}
}

func TestParseStringArray(t *testing.T) {
	t.Parallel()

	got, err := parseStringArray("[\"a\", \"b\", \"c\"]")
	if err != nil {
		t.Fatalf("parseStringArray() error = %v", err)
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("parseStringArray() = %#v", got)
	}
}

func TestParseSleepDurationRequiresDurationMS(t *testing.T) {
	t.Parallel()

	if _, err := parseSleepDuration(scriptTable{}); err == nil {
		t.Fatal("parseSleepDuration() error = nil, want missing duration_ms error")
	}
}

func TestParseSleepDurationRejectsNonPositiveValue(t *testing.T) {
	t.Parallel()

	if _, err := parseSleepDuration(scriptTable{
		"duration_ms": {kind: scriptNumber, number: 0},
	}); err == nil {
		t.Fatal("parseSleepDuration() error = nil, want non-positive duration_ms error")
	}
}

func TestExecuteScriptSleepOutputsProgress(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	var out bytes.Buffer

	sawGoAway, err := executeScript(h2c, scriptFile{
		actions: []scriptTable{
			{
				"type":        {kind: scriptString, str: "sleep"},
				"duration_ms": {kind: scriptNumber, number: 1},
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("executeScript() error = %v", err)
	}
	if sawGoAway {
		t.Fatal("executeScript() sawGoAway = true, want false")
	}
	if !strings.Contains(out.String(), ">> SLEEP 1ms") {
		t.Fatalf("output = %q, want sleep progress line", out.String())
	}
}

type nopConn struct{}

func (nopConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (nopConn) Write(p []byte) (int, error) { return len(p), nil }
func (nopConn) Close() error                { return nil }
