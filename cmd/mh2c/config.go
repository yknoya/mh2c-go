package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const defaultMaxDynamicTableSize uint = 8192

type stringFlags []string

func (s *stringFlags) String() string {
	return strings.Join(*s, ",")
}

func (s *stringFlags) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type headerFlags = stringFlags

type optionalUintFlag struct {
	value uint
	set   bool
}

func (f *optionalUintFlag) String() string {
	if !f.set {
		return ""
	}
	return strconv.FormatUint(uint64(f.value), 10)
}

func (f *optionalUintFlag) Set(src string) error {
	value, err := strconv.ParseUint(src, 10, strconv.IntSize)
	if err != nil {
		return err
	}
	f.value = uint(value)
	f.set = true
	return nil
}

type config struct {
	mode             string
	scriptCommand    string
	scriptFile       string
	scriptDescribe   string
	scriptTemplate   string
	rawURL           string
	scheme           string
	host             string
	authority        string
	path             string
	method           string
	data             string
	bodyFile         string
	pingData         string
	timeout          time.Duration
	maxTable         uint
	port             uint
	streamID         uint
	maxRecv          uint
	streamFilter     uint
	hasStreamFilter  bool
	insecure         bool
	sendGoAway       bool
	outputFormat     string
	dataFormat       string
	dataLimit        uint
	decodeHeaders    bool
	showHeaderBlock  bool
	saveOutput       string
	saveBody         string
	saveHeaders      string
	headers          headerFlags
	frameFilters     stringFlags
	directionFilters stringFlags
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	cfg := defaultConfig()
	if len(args) == 0 {
		printTopLevelUsage(stderr)
		return config{}, fmt.Errorf("missing command: use request, ping, observe, or script")
	}
	if args[0] == "-h" || args[0] == "--help" {
		printTopLevelUsage(stderr)
		return config{}, flag.ErrHelp
	}
	if args[0] == "--mode" || strings.HasPrefix(args[0], "--mode=") {
		return config{}, fmt.Errorf("--mode has been replaced by subcommands; use mh2c request, mh2c ping, mh2c observe, or mh2c script run")
	}
	if strings.HasPrefix(args[0], "-") {
		return config{}, fmt.Errorf("missing command before option %q; use mh2c request, mh2c ping, mh2c observe, or mh2c script", args[0])
	}

	switch args[0] {
	case "request":
		cfg.mode = "request"
		return parseRequestConfig(cfg, args[1:], stderr)
	case "ping":
		cfg.mode = "ping"
		return parsePingConfig(cfg, args[1:], stderr)
	case "observe":
		cfg.mode = "observe"
		return parseObserveConfig(cfg, args[1:], stderr)
	case "script":
		cfg.mode = "script"
		return parseScriptConfig(cfg, args[1:], stderr)
	default:
		return config{}, fmt.Errorf("unknown command %q: use request, ping, observe, or script", args[0])
	}
}

func defaultConfig() config {
	return config{
		mode:            "request",
		scheme:          "https",
		host:            "nghttp2.org",
		path:            "/httpbin/headers",
		method:          "GET",
		pingData:        "mh2cping",
		timeout:         10 * time.Second,
		maxTable:        defaultMaxDynamicTableSize,
		port:            443,
		streamID:        1,
		sendGoAway:      true,
		outputFormat:    outputFormatText,
		dataFormat:      dataFormatBoth,
		decodeHeaders:   true,
		showHeaderBlock: true,
	}
}

func parseRequestConfig(cfg config, args []string, stderr io.Writer) (config, error) {
	fs := newCommandFlagSet("mh2c request", stderr)
	streamFilter := addExecutionFlags(fs, &cfg)
	addRequestFlags(fs, &cfg)
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return config{}, err
	}
	applyStreamFilter(&cfg, streamFilter)
	return validateExecutionConfig(cfg)
}

func parsePingConfig(cfg config, args []string, stderr io.Writer) (config, error) {
	fs := newCommandFlagSet("mh2c ping", stderr)
	streamFilter := addExecutionFlags(fs, &cfg)
	fs.StringVar(&cfg.pingData, "ping-data", cfg.pingData, "8-byte ping payload")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return config{}, err
	}
	applyStreamFilter(&cfg, streamFilter)
	return validateExecutionConfig(cfg)
}

func parseObserveConfig(cfg config, args []string, stderr io.Writer) (config, error) {
	fs := newCommandFlagSet("mh2c observe", stderr)
	streamFilter := addExecutionFlags(fs, &cfg)
	addCaptureFlags(fs, &cfg)
	fs.UintVar(&cfg.maxRecv, "max-recv", 0, "maximum received frames; 0 means unlimited")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if err := rejectPositionalArgs(fs); err != nil {
		return config{}, err
	}
	applyStreamFilter(&cfg, streamFilter)
	return validateExecutionConfig(cfg)
}

func parseScriptConfig(cfg config, args []string, stderr io.Writer) (config, error) {
	if len(args) == 0 {
		printScriptUsage(stderr)
		return config{}, fmt.Errorf("missing script command: use run, describe, template, or validate")
	}
	if args[0] == "-h" || args[0] == "--help" {
		printScriptUsage(stderr)
		return config{}, flag.ErrHelp
	}

	switch args[0] {
	case "run":
		cfg.scriptCommand = "run"
		fs := newCommandFlagSet("mh2c script run", stderr)
		streamFilter := addExecutionFlags(fs, &cfg)
		fs.StringVar(&cfg.scriptFile, "script-file", "", "script file path")
		if err := fs.Parse(args[1:]); err != nil {
			return config{}, err
		}
		if err := rejectPositionalArgs(fs); err != nil {
			return config{}, err
		}
		applyStreamFilter(&cfg, streamFilter)
		if cfg.scriptFile == "" {
			return config{}, fmt.Errorf("script-file is required")
		}
		return validateExecutionConfig(cfg)
	case "describe":
		cfg.scriptCommand = "describe"
		fs := newCommandFlagSet("mh2c script describe", stderr)
		fs.StringVar(&cfg.scriptDescribe, "type", "", "show details for one action type")
		if err := fs.Parse(args[1:]); err != nil {
			return config{}, err
		}
		if err := rejectPositionalArgs(fs); err != nil {
			return config{}, err
		}
		return cfg, nil
	case "template":
		cfg.scriptCommand = "template"
		fs := newCommandFlagSet("mh2c script template", stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return config{}, err
		}
		remaining := fs.Args()
		if len(remaining) != 1 {
			return config{}, fmt.Errorf("script template requires exactly one template name")
		}
		cfg.scriptTemplate = remaining[0]
		return cfg, nil
	case "validate":
		cfg.scriptCommand = "validate"
		fs := newCommandFlagSet("mh2c script validate", stderr)
		fs.StringVar(&cfg.scriptFile, "script-file", "", "script file path")
		if err := fs.Parse(args[1:]); err != nil {
			return config{}, err
		}
		if err := rejectPositionalArgs(fs); err != nil {
			return config{}, err
		}
		if cfg.scriptFile == "" {
			return config{}, fmt.Errorf("script-file is required")
		}
		return cfg, nil
	default:
		return config{}, fmt.Errorf("unknown script command %q: use run, describe, template, or validate", args[0])
	}
}

func newCommandFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func rejectPositionalArgs(fs *flag.FlagSet) error {
	if fs.NArg() == 0 {
		return nil
	}
	return fmt.Errorf("%s does not accept positional arguments: %s", fs.Name(), strings.Join(fs.Args(), " "))
}

func addExecutionFlags(fs *flag.FlagSet, cfg *config) *optionalUintFlag {
	streamFilter := &optionalUintFlag{}

	fs.StringVar(&cfg.rawURL, "url", "", "target URL; when set, overrides scheme/host/port/path")
	fs.StringVar(&cfg.scheme, "scheme", cfg.scheme, "target scheme; only https is supported")
	fs.StringVar(&cfg.host, "host", cfg.host, "target host")
	fs.UintVar(&cfg.port, "port", cfg.port, "target port")
	fs.StringVar(&cfg.authority, "authority", "", "override :authority header")
	fs.StringVar(&cfg.path, "path", cfg.path, "request path")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "overall timeout")
	fs.UintVar(&cfg.maxTable, "max-table-size", cfg.maxTable, "initial HPACK dynamic table size")
	fs.UintVar(&cfg.streamID, "stream-id", cfg.streamID, "request stream ID")
	fs.Var(streamFilter, "stream-filter", "display and capture a specific stream ID")
	fs.BoolVar(&cfg.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&cfg.sendGoAway, "send-goaway", cfg.sendGoAway, "send GOAWAY before exit when peer did not")
	fs.StringVar(&cfg.outputFormat, "output", cfg.outputFormat, "output format: text or jsonl")
	fs.StringVar(&cfg.dataFormat, "data-format", cfg.dataFormat, "payload format: text, hex, or both")
	fs.UintVar(&cfg.dataLimit, "data-limit", 0, "truncate payload display to the first N bytes; 0 means unlimited")
	fs.BoolVar(&cfg.decodeHeaders, "decode-headers", cfg.decodeHeaders, "decode HPACK header blocks")
	fs.BoolVar(&cfg.showHeaderBlock, "show-header-block", cfg.showHeaderBlock, "show HPACK/header block fragments")
	fs.StringVar(&cfg.saveOutput, "save-output", "", "write the displayed CLI output to a file as well")
	fs.Var(&cfg.frameFilters, "frame-filter", "only display specific frame types; repeatable")
	fs.Var(&cfg.directionFilters, "direction-filter", "only display sent or received events; repeatable")
	return streamFilter
}

func addRequestFlags(fs *flag.FlagSet, cfg *config) {
	addCaptureFlags(fs, cfg)
	fs.StringVar(&cfg.method, "method", cfg.method, "HTTP method")
	fs.StringVar(&cfg.data, "data", "", "request body as a literal string")
	fs.StringVar(&cfg.bodyFile, "body-file", "", "request body file path or '-' for stdin")
	fs.Var(&cfg.headers, "header", "request header in 'name:value' format; repeatable")
}

func addCaptureFlags(fs *flag.FlagSet, cfg *config) {
	fs.StringVar(&cfg.saveBody, "save-body", "", "save the captured response body to a file")
	fs.StringVar(&cfg.saveHeaders, "save-headers", "", "save the decoded response headers to a file")
}

func applyStreamFilter(cfg *config, streamFilter *optionalUintFlag) {
	cfg.streamFilter = streamFilter.value
	cfg.hasStreamFilter = streamFilter.set
}

func validateExecutionConfig(cfg config) (config, error) {
	if cfg.streamID == 0 {
		return config{}, fmt.Errorf("stream-id must be greater than 0")
	}
	if cfg.maxTable > uint(^uint32(0)) {
		return config{}, fmt.Errorf("max-table-size %d exceeds uint32", cfg.maxTable)
	}
	if cfg.port > 65535 {
		return config{}, fmt.Errorf("port %d is out of range", cfg.port)
	}
	if cfg.outputFormat != outputFormatText && cfg.outputFormat != outputFormatJSONL {
		return config{}, fmt.Errorf("invalid output %q: want text or jsonl", cfg.outputFormat)
	}
	if cfg.dataFormat != dataFormatText && cfg.dataFormat != dataFormatHex && cfg.dataFormat != dataFormatBoth {
		return config{}, fmt.Errorf("invalid data-format %q: want text, hex, or both", cfg.dataFormat)
	}
	if _, err := buildFrameFilterSet(cfg.frameFilters); err != nil {
		return config{}, err
	}
	if err := validateDirectionFilters(cfg.directionFilters); err != nil {
		return config{}, err
	}
	if (cfg.saveBody != "" || cfg.saveHeaders != "") && cfg.mode != "request" && cfg.mode != "observe" {
		return config{}, fmt.Errorf("save-body and save-headers are only supported in request or observe mode")
	}
	return cfg, nil
}

func printTopLevelUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mh2c request [options]")
	fmt.Fprintln(w, "  mh2c ping [options]")
	fmt.Fprintln(w, "  mh2c observe [options]")
	fmt.Fprintln(w, "  mh2c script run --script-file file.toml [options]")
	fmt.Fprintln(w, "  mh2c script describe [--type action_type]")
	fmt.Fprintln(w, "  mh2c script template request")
	fmt.Fprintln(w, "  mh2c script validate --script-file file.toml")
}

func printScriptUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mh2c script run --script-file file.toml [options]")
	fmt.Fprintln(w, "  mh2c script describe [--type action_type]")
	fmt.Fprintln(w, "  mh2c script template request")
	fmt.Fprintln(w, "  mh2c script validate --script-file file.toml")
}
