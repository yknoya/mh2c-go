package main

import (
	"bytes"
	"encoding/hex"
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

func TestParseScriptWithRepeatFields(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "headers"
stream_id = 1
stream_id_step = 2
repeat = 3
flags = ["END_HEADERS", "END_STREAM"]
headers = [
  ":method: GET",
  ":path: /",
]
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	if got, ok, err := script.actions[0].intValue("repeat"); err != nil || !ok || got != 3 {
		t.Fatalf("repeat = %d, ok = %t, err = %v", got, ok, err)
	}
	if got, ok, err := script.actions[0].intValue("stream_id_step"); err != nil || !ok || got != 2 {
		t.Fatalf("stream_id_step = %d, ok = %t, err = %v", got, ok, err)
	}
}

func TestParseScriptAcceptsTOMLStringArrays(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = 'headers'
stream_id = 1
flags = [
  'END_HEADERS',
  'END_STREAM',
]
headers = [
  ':method: GET',
  ':path: /',
]
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	flags, ok, err := script.actions[0].stringListValue("flags")
	if err != nil || !ok || strings.Join(flags, ",") != "END_HEADERS,END_STREAM" {
		t.Fatalf("flags = %#v, ok = %t, err = %v", flags, ok, err)
	}
	headers, ok, err := script.actions[0].stringListValue("headers")
	if err != nil || !ok || strings.Join(headers, ",") != ":method: GET,:path: /" {
		t.Fatalf("headers = %#v, ok = %t, err = %v", headers, ok, err)
	}
}

func TestParseScriptRejectsNonStringArray(t *testing.T) {
	t.Parallel()

	_, err := parseScript(`
[[action]]
type = "headers"
stream_id = 1
flags = ["END_HEADERS", 1]
`)
	if err == nil || !strings.Contains(err.Error(), "array elements must be strings") {
		t.Fatalf("parseScript() error = %v, want string array validation", err)
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
	if typed.Header().StreamID != 1 || typed.Header().Flags != frame.FlagHeadersEndHeaders|frame.FlagHeadersEndStream || len(typed.BlockFragment) == 0 {
		t.Fatalf("HeadersFrame = %#v", typed)
	}
}

func TestBuildScriptFramePushPromiseWithBlockHex(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	got, err := buildScriptFrame(h2c, scriptTable{
		"type":               {kind: scriptString, str: "push_promise"},
		"stream_id":          {kind: scriptNumber, number: 1},
		"promised_stream_id": {kind: scriptNumber, number: 2},
		"flags":              {kind: scriptStringList, list: []string{"END_HEADERS"}},
		"block_hex":          {kind: scriptString, str: "8286"},
	})
	if err != nil {
		t.Fatalf("buildScriptFrame() error = %v", err)
	}

	typed, ok := got.(frame.PushPromiseFrame)
	if !ok {
		t.Fatalf("frame type = %T, want PushPromiseFrame", got)
	}
	if typed.Header().StreamID != 1 || typed.PromisedStreamID != 2 || typed.Header().Flags != frame.FlagPushPromiseEndHeaders {
		t.Fatalf("PushPromiseFrame = %#v", typed)
	}
	if gotHex := hex.EncodeToString(typed.BlockFragment); gotHex != "8286" {
		t.Fatalf("block fragment = %s, want 8286", gotHex)
	}
}

func TestBuildScriptFrameDataWithPaddingSetsHeaderLength(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	got, err := buildScriptFrame(h2c, scriptTable{
		"type":       {kind: scriptString, str: "data"},
		"stream_id":  {kind: scriptNumber, number: 1},
		"flags":      {kind: scriptStringList, list: []string{"PADDED"}},
		"data":       {kind: scriptString, str: "hi"},
		"pad_length": {kind: scriptNumber, number: 2},
	})
	if err != nil {
		t.Fatalf("buildScriptFrame() error = %v", err)
	}

	typed, ok := got.(frame.DataFrame)
	if !ok {
		t.Fatalf("frame type = %T, want DataFrame", got)
	}
	if typed.FrameHeader.Type != frame.TypeData || typed.FrameHeader.Length != 5 {
		t.Fatalf("FrameHeader = %#v", typed.FrameHeader)
	}
}

func TestBuildScriptFrameRaw(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	got, err := buildScriptFrame(h2c, scriptTable{
		"type":        {kind: scriptString, str: "raw"},
		"frame_type":  {kind: scriptNumber, number: 254},
		"stream_id":   {kind: scriptNumber, number: 1},
		"flags":       {kind: scriptNumber, number: 3},
		"payload_hex": {kind: scriptString, str: "deadbeef"},
	})
	if err != nil {
		t.Fatalf("buildScriptFrame() error = %v", err)
	}

	typed, ok := got.(frame.RawFrame)
	if !ok {
		t.Fatalf("frame type = %T, want RawFrame", got)
	}
	if typed.Header().Type != frame.Type(254) || typed.Header().Flags != 3 || typed.Header().StreamID != 1 {
		t.Fatalf("RawFrame header = %#v", typed.Header())
	}
	if gotHex := hex.EncodeToString(typed.Payload()); gotHex != "deadbeef" {
		t.Fatalf("payload = %s, want deadbeef", gotHex)
	}
}

func TestBuildScriptFrameRawWithExactLength(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	got, err := buildScriptFrame(h2c, scriptTable{
		"type":        {kind: scriptString, str: "raw"},
		"frame_type":  {kind: scriptNumber, number: 254},
		"stream_id":   {kind: scriptNumber, number: 1},
		"flags":       {kind: scriptNumber, number: 3},
		"length":      {kind: scriptNumber, number: 1},
		"payload_hex": {kind: scriptString, str: "deadbeef"},
	})
	if err != nil {
		t.Fatalf("buildScriptFrame() error = %v", err)
	}

	raw, err := got.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := frame.ParseHeader(raw[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	if header.Length != 1 {
		t.Fatalf("Header.Length = %d, want 1", header.Length)
	}
	if gotHex := hex.EncodeToString(raw[9:]); gotHex != "deadbeef" {
		t.Fatalf("payload = %s, want deadbeef", gotHex)
	}
}

func TestClientFrameEventTracksHeaderBlockCompleteForDisplay(t *testing.T) {
	t.Parallel()

	h2c := client.NewWithConn(nopConn{}, client.WithMaxDynamicTableSize(4096))
	block, err := h2c.ResponseCodec().Encode([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	event := h2c.TrackReceivedFrame(frame.NewHeadersFrame(1, 0, block[:len(block)/2]))
	if event.DecodeError != nil {
		t.Fatalf("TrackReceivedFrame(HEADERS) DecodeError = %v", event.DecodeError)
	}
	if event.HeaderBlockComplete || len(event.Headers) != 0 || event.HeaderBlockStreamID != 0 || event.HeaderBlockEndStream {
		t.Fatalf("HEADERS event = %#v", event)
	}
	event = h2c.TrackReceivedFrame(frame.NewContinuationFrame(1, frame.FlagContinuationEndHeaders, block[len(block)/2:]))
	if event.DecodeError != nil {
		t.Fatalf("TrackReceivedFrame(CONTINUATION) DecodeError = %v", event.DecodeError)
	}
	if event.HeaderBlockStreamID != 1 || len(event.Warnings) != 0 || event.HeaderBlockEndStream || fieldValue(event.Headers, ":status") != "200" {
		t.Fatalf("event = %#v", event)
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
	controller, err := newOutputController(&out, config{
		mode:            "script",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	sawGoAway, err := executeScript(h2c, scriptFile{
		actions: []scriptTable{
			{
				"type":        {kind: scriptString, str: "sleep"},
				"duration_ms": {kind: scriptNumber, number: 1},
			},
		},
	}, controller)
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

func TestExecuteScriptRepeatsWindowUpdate(t *testing.T) {
	t.Parallel()

	conn := &scriptedConn{}
	h2c := client.NewWithConn(conn, client.WithMaxDynamicTableSize(4096))
	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "script",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	sawGoAway, err := executeScript(h2c, scriptFile{
		actions: []scriptTable{
			{
				"type":      {kind: scriptString, str: "window_update"},
				"stream_id": {kind: scriptNumber, number: 0},
				"increment": {kind: scriptNumber, number: 1024},
				"repeat":    {kind: scriptNumber, number: 10},
			},
		},
	}, controller)
	if err != nil {
		t.Fatalf("executeScript() error = %v", err)
	}
	if sawGoAway {
		t.Fatal("executeScript() sawGoAway = true, want false")
	}
	const frameSize = 13
	if got, want := conn.writes.Len(), 10*frameSize; got != want {
		t.Fatalf("written bytes = %d, want %d", got, want)
	}
	for i := 0; i < 10; i++ {
		header, err := frame.ParseHeader(conn.writes.Bytes()[i*frameSize : i*frameSize+9])
		if err != nil {
			t.Fatalf("ParseHeader(%d) error = %v", i, err)
		}
		if header.Type != frame.TypeWindowUpdate || header.StreamID != 0 || header.Length != 4 {
			t.Fatalf("header %d = %#v, want WINDOW_UPDATE stream 0 length 4", i, header)
		}
	}
	if got := strings.Count(out.String(), ">> WINDOW_UPDATE stream=0"); got != 10 {
		t.Fatalf("WINDOW_UPDATE output count = %d, want 10; output = %q", got, out.String())
	}
}

func TestExecuteScriptRepeatAdvancesStreamIDForHeaders(t *testing.T) {
	t.Parallel()

	conn := &scriptedConn{}
	h2c := client.NewWithConn(conn, client.WithMaxDynamicTableSize(4096))
	var out bytes.Buffer
	controller, err := newOutputController(&out, config{
		mode:            "script",
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	})
	if err != nil {
		t.Fatalf("newOutputController() error = %v", err)
	}

	sawGoAway, err := executeScript(h2c, scriptFile{
		actions: []scriptTable{
			{
				"type":           {kind: scriptString, str: "headers"},
				"stream_id":      {kind: scriptNumber, number: 1},
				"stream_id_step": {kind: scriptNumber, number: 2},
				"repeat":         {kind: scriptNumber, number: 3},
				"flags":          {kind: scriptStringList, list: []string{"END_HEADERS", "END_STREAM"}},
				"headers":        {kind: scriptStringList, list: []string{":method: GET", ":path: /"}},
			},
		},
	}, controller)
	if err != nil {
		t.Fatalf("executeScript() error = %v", err)
	}
	if sawGoAway {
		t.Fatal("executeScript() sawGoAway = true, want false")
	}

	raw := conn.writes.Bytes()
	offset := 0
	for i, wantStreamID := range []uint32{1, 3, 5} {
		if offset+9 > len(raw) {
			t.Fatalf("frame %d offset %d exceeds written bytes %d", i, offset, len(raw))
		}
		header, err := frame.ParseHeader(raw[offset : offset+9])
		if err != nil {
			t.Fatalf("ParseHeader(%d) error = %v", i, err)
		}
		if header.Type != frame.TypeHeaders || header.StreamID != wantStreamID {
			t.Fatalf("header %d = %#v, want HEADERS stream %d", i, header, wantStreamID)
		}
		offset += 9 + int(header.Length)
	}
	if offset != len(raw) {
		t.Fatalf("parsed bytes = %d, written bytes = %d", offset, len(raw))
	}
	text := out.String()
	for _, want := range []string{">> HEADERS stream=1", ">> HEADERS stream=3", ">> HEADERS stream=5"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}

func TestDescribeScriptActionsIncludesKnownTypes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := describeScriptActions(&out, ""); err != nil {
		t.Fatalf("describeScriptActions() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"headers", "receive", "raw"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}

func TestDescribeScriptActionHeaders(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := describeScriptActions(&out, "headers"); err != nil {
		t.Fatalf("describeScriptActions() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"Action: headers", "stream_id", "headers", "block_hex", "repeat", "stream_id_step", "END_HEADERS"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}

func TestWriteScriptTemplateRequestParses(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := writeScriptTemplate(&out, "request"); err != nil {
		t.Fatalf("writeScriptTemplate() error = %v", err)
	}
	script, err := parseScript(out.String())
	if err != nil {
		t.Fatalf("parseScript(template) error = %v", err)
	}
	if err := validateScript(script); err != nil {
		t.Fatalf("validateScript(template) error = %v", err)
	}
}

func TestValidateScriptRejectsUnknownActionType(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "unknown"
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), `unsupported action type "unknown"`) {
		t.Fatalf("validateScript() error = %v, want unknown action type", err)
	}
}

func TestValidateScriptRejectsMissingRequiredField(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "raw"
stream_id = 1
flags = 0
payload_hex = "00"
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "action.frame_type is required") {
		t.Fatalf("validateScript() error = %v, want missing frame_type", err)
	}
}

func TestValidateScriptRejectsUnknownActionField(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "preface"
extra = "value"
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "action.extra is not supported") {
		t.Fatalf("validateScript() error = %v, want unknown field", err)
	}
}

func TestValidateScriptRejectsInvalidRepeat(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "preface"
repeat = 0
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "repeat must be > 0") {
		t.Fatalf("validateScript() error = %v, want repeat validation", err)
	}
}

func TestValidateScriptRejectsStreamIDStepWithoutRepeat(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "headers"
stream_id = 1
stream_id_step = 2
flags = ["END_HEADERS"]
headers = [
  ":method: GET",
  ":path: /",
]
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "stream_id_step requires repeat") {
		t.Fatalf("validateScript() error = %v, want stream_id_step repeat validation", err)
	}
}

func TestValidateScriptRejectsStreamIDStepWithoutStreamID(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "ping"
repeat = 2
stream_id_step = 2
data = "abcdefgh"
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "stream_id_step requires an action with stream_id") {
		t.Fatalf("validateScript() error = %v, want stream_id_step stream_id validation", err)
	}
}

func TestValidateScriptRejectsStreamIDStepOverflow(t *testing.T) {
	t.Parallel()

	script, err := parseScript(`
[[action]]
type = "window_update"
stream_id = 4294967295
stream_id_step = 1
repeat = 2
increment = 1024
`)
	if err != nil {
		t.Fatalf("parseScript() error = %v", err)
	}
	err = validateScript(script)
	if err == nil || !strings.Contains(err.Error(), "stream_id_step overflows stream_id") {
		t.Fatalf("validateScript() error = %v, want stream_id overflow validation", err)
	}
}

type nopConn struct{}

func (nopConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (nopConn) Write(p []byte) (int, error) { return len(p), nil }
func (nopConn) Close() error                { return nil }
