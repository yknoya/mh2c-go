package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

const defaultMaxDynamicTableSize uint = 8192

type headerFlags []string

func (h *headerFlags) String() string {
	return strings.Join(*h, ",")
}

func (h *headerFlags) Set(v string) error {
	*h = append(*h, v)
	return nil
}

type config struct {
	mode       string
	scriptFile string
	rawURL     string
	scheme     string
	host       string
	authority  string
	path       string
	method     string
	data       string
	bodyFile   string
	pingData   string
	timeout    time.Duration
	maxTable   uint
	port       uint
	streamID   uint
	insecure   bool
	sendGoAway bool
	headers    headerFlags
}

type endpoint struct {
	scheme    string
	host      string
	authority string
	path      string
	port      uint16
}

type responseState struct {
	streamID           uint32
	pendingStreamID    uint32
	pendingHeaderBlock []byte
	pendingEndStream   bool
}

type consumeResult struct {
	headers  []hpack.HeaderField
	warnings []string
	data     []byte
	done     bool
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	var script scriptFile
	if cfg.mode == "script" {
		if cfg.scriptFile == "" {
			return fmt.Errorf("script-file is required when mode=script")
		}
		script, err = parseScriptFile(cfg.scriptFile)
		if err != nil {
			return err
		}
		cfg, err = applyScriptConnection(cfg, script)
		if err != nil {
			return err
		}
	}
	ep, err := resolveEndpoint(cfg)
	if err != nil {
		return err
	}

	ctx := parent
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, cfg.timeout)
		defer cancel()
	}

	opts := []client.Option{client.WithMaxDynamicTableSize(uint32(cfg.maxTable))}
	if cfg.insecure {
		opts = append(opts, client.WithInsecureSkipVerify())
	}

	h2c, err := client.New(ctx, ep.host, ep.port, opts...)
	if err != nil {
		return err
	}
	defer h2c.Close()

	streamID := uint32(cfg.streamID)
	sawGoAway := false
	switch cfg.mode {
	case "request":
		if err := startSession(h2c, uint32(cfg.maxTable), stdout); err != nil {
			return err
		}
		body, err := readRequestBody(cfg, stdin)
		if err != nil {
			return err
		}
		fields, err := buildRequestFields(ep, cfg, body)
		if err != nil {
			return err
		}
		if err := sendRequest(h2c, streamID, fields, body); err != nil {
			return err
		}
		sawGoAway, err = receiveResponseFrames(h2c, streamID, stdout)
		if err != nil {
			return err
		}
	case "ping":
		if err := startSession(h2c, uint32(cfg.maxTable), stdout); err != nil {
			return err
		}
		payload, err := parsePingData(cfg.pingData)
		if err != nil {
			return err
		}
		if err := h2c.SendFrame(frame.PingFrame{Data: payload}); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "sent PING payload=%q hex=%s\n", string(payload[:]), hex.EncodeToString(payload[:]))
		sawGoAway, err = receivePingFrames(h2c, payload, stdout)
		if err != nil {
			return err
		}
	case "script":
		sawGoAway, err = executeScript(h2c, script, stdout)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}

	if cfg.sendGoAway && !sawGoAway {
		_ = h2c.SendFrame(frame.GoAwayFrame{
			LastStreamID: streamID,
			ErrorCode:    frame.ErrNo,
		})
	}
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("mh2c", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := config{
		mode:       "request",
		scheme:     "https",
		host:       "nghttp2.org",
		path:       "/httpbin/headers",
		method:     "GET",
		pingData:   "mh2cping",
		timeout:    10 * time.Second,
		maxTable:   defaultMaxDynamicTableSize,
		port:       443,
		streamID:   1,
		sendGoAway: true,
	}

	fs.StringVar(&cfg.mode, "mode", cfg.mode, "operation mode: request, ping, or script")
	fs.StringVar(&cfg.scriptFile, "script-file", "", "script file path for mode=script")
	fs.StringVar(&cfg.rawURL, "url", "", "target URL; when set, overrides scheme/host/port/path")
	fs.StringVar(&cfg.scheme, "scheme", cfg.scheme, "target scheme; only https is supported")
	fs.StringVar(&cfg.host, "host", cfg.host, "target host")
	fs.UintVar(&cfg.port, "port", cfg.port, "target port")
	fs.StringVar(&cfg.authority, "authority", "", "override :authority header")
	fs.StringVar(&cfg.path, "path", cfg.path, "request path")
	fs.StringVar(&cfg.method, "method", cfg.method, "HTTP method")
	fs.StringVar(&cfg.data, "data", "", "request body as a literal string")
	fs.StringVar(&cfg.bodyFile, "body-file", "", "request body file path or '-' for stdin")
	fs.StringVar(&cfg.pingData, "ping-data", cfg.pingData, "8-byte ping payload")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "overall timeout")
	fs.UintVar(&cfg.maxTable, "max-table-size", cfg.maxTable, "initial HPACK dynamic table size")
	fs.UintVar(&cfg.streamID, "stream-id", cfg.streamID, "request stream ID")
	fs.BoolVar(&cfg.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&cfg.sendGoAway, "send-goaway", cfg.sendGoAway, "send GOAWAY before exit when peer did not")
	fs.Var(&cfg.headers, "header", "request header in 'name:value' format; repeatable")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.mode != "request" && cfg.mode != "ping" && cfg.mode != "script" {
		return config{}, fmt.Errorf("invalid mode %q: want request, ping, or script", cfg.mode)
	}
	if cfg.streamID == 0 {
		return config{}, fmt.Errorf("stream-id must be greater than 0")
	}
	if cfg.maxTable > uint(^uint32(0)) {
		return config{}, fmt.Errorf("max-table-size %d exceeds uint32", cfg.maxTable)
	}
	if cfg.port > 65535 {
		return config{}, fmt.Errorf("port %d is out of range", cfg.port)
	}
	return cfg, nil
}

func resolveEndpoint(cfg config) (endpoint, error) {
	if cfg.rawURL != "" {
		u, err := url.Parse(cfg.rawURL)
		if err != nil {
			return endpoint{}, err
		}
		if !u.IsAbs() {
			return endpoint{}, fmt.Errorf("url %q must be absolute", cfg.rawURL)
		}
		if u.Scheme != "https" {
			return endpoint{}, fmt.Errorf("scheme %q is not supported; only https is supported", u.Scheme)
		}
		host := u.Hostname()
		if host == "" {
			return endpoint{}, fmt.Errorf("url %q does not contain a host", cfg.rawURL)
		}
		port := uint64(443)
		if u.Port() != "" {
			parsed, err := strconv.ParseUint(u.Port(), 10, 16)
			if err != nil {
				return endpoint{}, err
			}
			port = parsed
		}
		path := u.RequestURI()
		if path == "" {
			path = "/"
		}
		authority := u.Host
		if cfg.authority != "" {
			authority = cfg.authority
		}
		return endpoint{
			scheme:    u.Scheme,
			host:      host,
			authority: authority,
			path:      path,
			port:      uint16(port),
		}, nil
	}

	if cfg.scheme != "https" {
		return endpoint{}, fmt.Errorf("scheme %q is not supported; only https is supported", cfg.scheme)
	}
	path := cfg.path
	if path == "" {
		path = "/"
	}
	authority := cfg.authority
	if authority == "" {
		if cfg.port == 443 {
			authority = cfg.host
		} else {
			authority = net.JoinHostPort(cfg.host, strconv.Itoa(int(cfg.port)))
		}
	}
	return endpoint{
		scheme:    cfg.scheme,
		host:      cfg.host,
		authority: authority,
		path:      path,
		port:      uint16(cfg.port),
	}, nil
}

func startSession(h2c *client.Client, maxTable uint32, out io.Writer) error {
	if err := h2c.SendConnectionPreface(); err != nil {
		return err
	}
	if err := h2c.SendFrame(frame.SettingsFrame{
		Settings: []frame.Setting{
			{ID: frame.SettingEnablePush, Value: 0},
			{ID: frame.SettingInitialWindowSize, Value: 65535},
			{ID: frame.SettingHeaderTableSize, Value: maxTable},
		},
	}); err != nil {
		return err
	}
	h2c.RequestCodec().SetMaxDynamicTableSize(maxTable)

	for {
		f, err := h2c.ReceiveFrame()
		if err != nil {
			return err
		}
		printReceivedFrame(out, h2c, f)

		switch typed := f.(type) {
		case frame.SettingsFrame:
			if typed.Flags&frame.FlagSettingsAck != 0 {
				continue
			}
			return h2c.SendFrame(frame.SettingsFrame{Flags: frame.FlagSettingsAck})
		case frame.PingFrame:
			if typed.Flags&frame.FlagPingAck == 0 {
				if err := h2c.SendFrame(frame.PingFrame{Flags: frame.FlagPingAck, Data: typed.Data}); err != nil {
					return err
				}
			}
		case frame.GoAwayFrame:
			return nil
		}
	}
}

func readRequestBody(cfg config, stdin io.Reader) ([]byte, error) {
	if cfg.data != "" && cfg.bodyFile != "" {
		return nil, fmt.Errorf("data and body-file cannot be used together")
	}
	switch {
	case cfg.bodyFile == "":
		return []byte(cfg.data), nil
	case cfg.bodyFile == "-":
		return io.ReadAll(stdin)
	default:
		return os.ReadFile(cfg.bodyFile)
	}
}

func buildRequestFields(ep endpoint, cfg config, body []byte) ([]hpack.HeaderField, error) {
	customFields := make([]hpack.HeaderField, 0, len(cfg.headers))
	overrides := map[string]bool{}
	for _, raw := range cfg.headers {
		field, err := parseHeader(raw)
		if err != nil {
			return nil, err
		}
		customFields = append(customFields, field)
		overrides[field.Name] = true
	}

	fields := make([]hpack.HeaderField, 0, 6+len(customFields))
	defaults := []hpack.HeaderField{
		{Name: ":method", Value: cfg.method},
		{Name: ":path", Value: ep.path},
		{Name: ":scheme", Value: ep.scheme},
		{Name: ":authority", Value: ep.authority},
	}
	for _, field := range defaults {
		if !overrides[field.Name] {
			fields = append(fields, field)
		}
	}
	if len(body) > 0 && !overrides["content-length"] {
		fields = append(fields, hpack.HeaderField{
			Name:  "content-length",
			Value: strconv.Itoa(len(body)),
		})
	}
	fields = append(fields, customFields...)
	return fields, nil
}

func parseHeader(raw string) (hpack.HeaderField, error) {
	sep := strings.Index(raw, ":")
	if strings.HasPrefix(raw, ":") {
		next := strings.Index(raw[1:], ":")
		if next >= 0 {
			sep = next + 1
		}
	}
	if sep <= 0 || sep >= len(raw)-1 {
		return hpack.HeaderField{}, fmt.Errorf("invalid header %q", raw)
	}
	name := strings.ToLower(strings.TrimSpace(raw[:sep]))
	value := strings.TrimSpace(raw[sep+1:])
	if name == "" {
		return hpack.HeaderField{}, fmt.Errorf("invalid header %q", raw)
	}
	return hpack.HeaderField{Name: name, Value: value}, nil
}

func sendRequest(h2c *client.Client, streamID uint32, fields []hpack.HeaderField, body []byte) error {
	block, err := h2c.EncodeHeaders(fields)
	if err != nil {
		return err
	}

	flags := uint8(frame.FlagHeadersEndHeaders)
	if len(body) == 0 {
		flags |= frame.FlagHeadersEndStream
	}
	if err := h2c.SendFrame(frame.HeadersFrame{
		StreamID:      streamID,
		Flags:         flags,
		BlockFragment: block,
	}); err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return h2c.SendFrame(frame.DataFrame{
		StreamID: streamID,
		Flags:    frame.FlagDataEndStream,
		Data:     body,
	})
}

func receiveResponseFrames(h2c *client.Client, streamID uint32, out io.Writer) (bool, error) {
	state := responseState{streamID: streamID}
	sawGoAway := false

	for {
		f, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		printReceivedFrame(out, h2c, f)

		switch typed := f.(type) {
		case frame.SettingsFrame:
			if typed.Flags&frame.FlagSettingsAck == 0 {
				if err := h2c.SendFrame(frame.SettingsFrame{Flags: frame.FlagSettingsAck}); err != nil {
					return sawGoAway, err
				}
			}
		case frame.PingFrame:
			if typed.Flags&frame.FlagPingAck == 0 {
				if err := h2c.SendFrame(frame.PingFrame{Flags: frame.FlagPingAck, Data: typed.Data}); err != nil {
					return sawGoAway, err
				}
			}
		case frame.GoAwayFrame:
			sawGoAway = true
			return sawGoAway, nil
		}

		result, err := state.Consume(f, h2c.DecodeHeadersDetailed)
		if err != nil {
			return sawGoAway, err
		}
		if len(result.warnings) > 0 {
			printHeaderWarnings(out, result.warnings)
		}
		if len(result.headers) > 0 {
			printHeaderFields(out, result.headers)
		}
		if result.done {
			return sawGoAway, nil
		}
	}
}

func receivePingFrames(h2c *client.Client, want [8]byte, out io.Writer) (bool, error) {
	sawGoAway := false
	for {
		f, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		printReceivedFrame(out, h2c, f)

		switch typed := f.(type) {
		case frame.SettingsFrame:
			if typed.Flags&frame.FlagSettingsAck == 0 {
				if err := h2c.SendFrame(frame.SettingsFrame{Flags: frame.FlagSettingsAck}); err != nil {
					return sawGoAway, err
				}
			}
		case frame.PingFrame:
			if typed.Flags&frame.FlagPingAck == 0 {
				if err := h2c.SendFrame(frame.PingFrame{Flags: frame.FlagPingAck, Data: typed.Data}); err != nil {
					return sawGoAway, err
				}
				continue
			}
			if typed.Data == want {
				return sawGoAway, nil
			}
		case frame.GoAwayFrame:
			sawGoAway = true
			return sawGoAway, nil
		}
	}
}

func (s *responseState) Consume(f frame.Frame, decode func([]byte) (hpack.DecodeReport, error)) (consumeResult, error) {
	switch typed := f.(type) {
	case frame.HeadersFrame:
		if typed.StreamID != s.streamID {
			return consumeResult{}, nil
		}
		if s.pendingStreamID != 0 {
			return consumeResult{}, fmt.Errorf("received HEADERS before previous header block finished on stream %d", s.pendingStreamID)
		}
		s.pendingStreamID = typed.StreamID
		s.pendingHeaderBlock = append([]byte(nil), typed.BlockFragment...)
		s.pendingEndStream = typed.Flags&frame.FlagHeadersEndStream != 0
		if typed.Flags&frame.FlagHeadersEndHeaders != 0 {
			return s.finishHeaderBlock(decode)
		}
	case frame.ContinuationFrame:
		if s.pendingStreamID == 0 {
			return consumeResult{}, fmt.Errorf("unexpected CONTINUATION frame on stream %d", typed.StreamID)
		}
		if typed.StreamID != s.pendingStreamID {
			return consumeResult{}, fmt.Errorf("CONTINUATION stream mismatch: got %d, want %d", typed.StreamID, s.pendingStreamID)
		}
		s.pendingHeaderBlock = append(s.pendingHeaderBlock, typed.BlockFragment...)
		if typed.Flags&frame.FlagContinuationEndHeaders != 0 {
			return s.finishHeaderBlock(decode)
		}
	case frame.DataFrame:
		if typed.StreamID != s.streamID {
			return consumeResult{}, nil
		}
		return consumeResult{
			data: append([]byte(nil), typed.Data...),
			done: typed.Flags&frame.FlagDataEndStream != 0,
		}, nil
	case frame.GoAwayFrame:
		return consumeResult{done: true}, nil
	}
	return consumeResult{}, nil
}

func (s *responseState) finishHeaderBlock(decode func([]byte) (hpack.DecodeReport, error)) (consumeResult, error) {
	report, err := decode(s.pendingHeaderBlock)
	if err != nil {
		return consumeResult{}, err
	}
	result := consumeResult{
		headers:  report.Fields,
		warnings: report.Warnings,
		done:     s.pendingEndStream,
	}
	s.pendingStreamID = 0
	s.pendingHeaderBlock = nil
	s.pendingEndStream = false
	return result, nil
}

func parsePingData(src string) ([8]byte, error) {
	var payload [8]byte
	if len(src) != len(payload) {
		return payload, fmt.Errorf("ping-data must be exactly 8 bytes, got %d", len(src))
	}
	copy(payload[:], src)
	return payload, nil
}

func printReceivedFrame(out io.Writer, c *client.Client, f frame.Frame) {
	fmt.Fprintf(out, "<< %s\n", client.DebugFrameString(f))

	switch typed := f.(type) {
	case frame.SettingsFrame:
		if len(typed.Settings) == 0 {
			fmt.Fprintln(out, "  settings: <empty>")
			return
		}
		for _, setting := range typed.Settings {
			fmt.Fprintf(out, "  setting id=%s value=%d\n", settingName(setting.ID), setting.Value)
		}
	case frame.HeadersFrame:
		fmt.Fprintf(out, "  header-block-fragment: %s\n", hex.EncodeToString(typed.BlockFragment))
	case frame.ContinuationFrame:
		fmt.Fprintf(out, "  continuation-fragment: %s\n", hex.EncodeToString(typed.BlockFragment))
	case frame.PushPromiseFrame:
		fmt.Fprintf(out, "  promised-stream-id: %d\n", typed.PromisedStreamID)
		fmt.Fprintf(out, "  header-block-fragment: %s\n", hex.EncodeToString(typed.BlockFragment))
		if typed.Flags&frame.FlagPushPromiseEndHeaders != 0 {
			printDecodedHeaders(out, c, typed.BlockFragment)
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
	case frame.RawFrame:
		payload := typed.Payload()
		fmt.Fprintf(out, "  raw-payload-hex: %s\n", hex.EncodeToString(payload))
	default:
		fmt.Fprintf(out, "  payload-hex: %s\n", hex.EncodeToString(f.Payload()))
	}
}

func printDecodedHeaders(out io.Writer, c *client.Client, block []byte) {
	report, err := c.DecodeHeadersDetailed(block)
	if err != nil {
		fmt.Fprintf(out, "  header-decode-error: %v\n", err)
		return
	}
	printHeaderWarnings(out, report.Warnings)
	printHeaderFields(out, report.Fields)
}

func printHeaderWarnings(out io.Writer, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(out, "  header-warning: %s\n", warning)
	}
}

func printHeaderFields(out io.Writer, fields []hpack.HeaderField) {
	for _, field := range fields {
		fmt.Fprintf(out, "  header %s: %s\n", field.Name, field.Value)
	}
}

func formatDataText(data []byte) string {
	if len(data) == 0 {
		return "<empty>"
	}
	if utf8.Valid(data) {
		return strconv.Quote(string(data))
	}
	return "<non-utf8>"
}

func settingName(id frame.SettingID) string {
	switch id {
	case frame.SettingHeaderTableSize:
		return "HEADER_TABLE_SIZE"
	case frame.SettingEnablePush:
		return "ENABLE_PUSH"
	case frame.SettingMaxConcurrentStreams:
		return "MAX_CONCURRENT_STREAMS"
	case frame.SettingInitialWindowSize:
		return "INITIAL_WINDOW_SIZE"
	case frame.SettingMaxFrameSize:
		return "MAX_FRAME_SIZE"
	case frame.SettingMaxHeaderListSize:
		return "MAX_HEADER_LIST_SIZE"
	default:
		return fmt.Sprintf("0x%04x", uint16(id))
	}
}
