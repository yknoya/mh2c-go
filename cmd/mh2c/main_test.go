package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestParseConfigObserveMode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg, err := parseConfig([]string{
		"observe",
		"--output", "jsonl",
		"--data-format", "hex",
		"--direction-filter", "received",
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
	if len(cfg.directionFilters) != 1 || cfg.directionFilters[0] != "received" {
		t.Fatalf("directionFilters = %#v", cfg.directionFilters)
	}
}

func TestParseConfigRequestCommand(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg, err := parseConfig([]string{
		"request",
		"--url", "https://example.com/demo",
		"--method", "POST",
		"--header", "content-type:application/json",
		"--data", "{}",
	}, &stderr)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.mode != "request" || cfg.rawURL != "https://example.com/demo" || cfg.method != "POST" || cfg.data != "{}" {
		t.Fatalf("parseConfig() = %#v", cfg)
	}
	if len(cfg.headers) != 1 || cfg.headers[0] != "content-type:application/json" {
		t.Fatalf("headers = %#v", cfg.headers)
	}
}

func TestParseConfigRejectsInvalidDirectionFilter(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	_, err := parseConfig([]string{
		"request",
		"--direction-filter", "outbound",
	}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "invalid direction-filter") {
		t.Fatalf("parseConfig() error = %v, want direction-filter validation", err)
	}
}

func TestParseConfigRejectsLegacyModeFlag(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	_, err := parseConfig([]string{
		"--mode", "script",
	}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--mode has been replaced by subcommands") {
		t.Fatalf("parseConfig() error = %v, want legacy mode error", err)
	}
}

func TestParseConfigScriptRun(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg, err := parseConfig([]string{
		"script", "run",
		"--script-file", "request.toml",
		"--direction-filter", "sent",
	}, &stderr)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.mode != "script" || cfg.scriptCommand != "run" || cfg.scriptFile != "request.toml" {
		t.Fatalf("parseConfig() = %#v", cfg)
	}
	if len(cfg.directionFilters) != 1 || cfg.directionFilters[0] != "sent" {
		t.Fatalf("directionFilters = %#v", cfg.directionFilters)
	}
}

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

func TestClientFrameEventConsumesContinuation(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	block, err := h2c.ResponseCodec().Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	split := len(block) / 2

	event := h2c.TrackReceivedFrame(frame.NewHeadersFrame(1, 0, block[:split]))
	if event.DecodeError != nil {
		t.Fatalf("TrackReceivedFrame(HEADERS) DecodeError = %v", event.DecodeError)
	}
	if event.HeaderBlockComplete || len(event.Headers) != 0 {
		t.Fatalf("TrackReceivedFrame(HEADERS) = %#v, want pending state", event)
	}

	event = h2c.TrackReceivedFrame(frame.NewContinuationFrame(1, frame.FlagContinuationEndHeaders, block[split:]))
	if event.DecodeError != nil {
		t.Fatalf("TrackReceivedFrame(CONTINUATION) DecodeError = %v", event.DecodeError)
	}
	if !event.HeaderBlockComplete || event.StreamID != 1 || fieldValue(event.Headers, ":status") != "200" {
		t.Fatalf("TrackReceivedFrame(CONTINUATION) = %#v, want decoded headers", event)
	}

	event = h2c.TrackReceivedFrame(frame.NewDataFrame(1, frame.FlagDataEndStream, []byte("hello")))
	if event.DecodeError != nil || event.HeaderBlockComplete {
		t.Fatalf("TrackReceivedFrame(DATA) = %#v, want non-header event", event)
	}
}

func TestPrepareOutputWriterMirrorsOutputToFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "mh2c.log")

	var stdout bytes.Buffer
	out, closeFn, err := prepareOutputWriter(&stdout, outputPath)
	if err != nil {
		t.Fatalf("prepareOutputWriter() error = %v", err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			t.Fatalf("closeFn() error = %v", err)
		}
	}()

	if _, err := io.WriteString(out, "line one\nline two\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}

	saved, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if stdout.String() != "line one\nline two\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if string(saved) != stdout.String() {
		t.Fatalf("saved output = %q, want %q", saved, stdout.String())
	}
}

func TestStartSessionDisplaysSentPrefaceAndSettings(t *testing.T) {
	t.Parallel()

	serverSettings, err := frame.NewSettingsFrame(0, []frame.Setting{
		{ID: frame.SettingMaxConcurrentStreams, Value: 100},
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	conn := &scriptedConn{reader: bytes.NewReader(serverSettings)}
	h2c := client.NewWithConn(conn, client.WithMaxDynamicTableSize(4096))

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

	if err := startSession(h2c, 8192, controller); err != nil {
		t.Fatalf("startSession() error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, ">> CONNECTION_PREFACE") {
		t.Fatalf("output = %q, want sent preface", text)
	}
	if !strings.Contains(text, ">> SETTINGS stream=0 len=18 type=SETTINGS(0x04) flags=0x00") ||
		!strings.Contains(text, "settings=[ENABLE_PUSH=0 INITIAL_WINDOW_SIZE=65535 HEADER_TABLE_SIZE=8192]") {
		t.Fatalf("output = %q, want sent initial SETTINGS", text)
	}
	if !strings.Contains(text, ">> SETTINGS stream=0 len=0 type=SETTINGS(0x04) flags=0x01 ack=true") {
		t.Fatalf("output = %q, want sent SETTINGS ack", text)
	}
}

func TestSendRequestDisplaysSentHeadersAndData(t *testing.T) {
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
		{Name: ":method", Value: "POST"},
		{Name: ":path", Value: "/demo"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: "content-length", Value: "5"},
	}

	if err := sendRequest(h2c, 1, fields, []byte("hello"), controller); err != nil {
		t.Fatalf("sendRequest() error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, ">> HEADERS stream=1") {
		t.Fatalf("output = %q, want sent HEADERS", text)
	}
	if !strings.Contains(text, ">> DATA stream=1 len=5 type=DATA(0x00) flags=0x01 end_stream=true data_bytes=5") {
		t.Fatalf("output = %q, want sent DATA", text)
	}
	if !strings.Contains(text, "data-text: \"hello\"") {
		t.Fatalf("output = %q, want displayed DATA payload", text)
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

type scriptedConn struct {
	reader *bytes.Reader
	writes bytes.Buffer
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	if c.reader == nil {
		return 0, io.EOF
	}
	return c.reader.Read(p)
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	return c.writes.Write(p)
}

func (*scriptedConn) Close() error {
	return nil
}
