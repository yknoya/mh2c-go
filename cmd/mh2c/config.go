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
	scriptFile       string
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
	fs := flag.NewFlagSet("mh2c", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var streamFilter optionalUintFlag

	cfg := config{
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

	fs.StringVar(&cfg.mode, "mode", cfg.mode, "operation mode: request, ping, script, or observe")
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
	fs.UintVar(&cfg.maxRecv, "max-recv", 0, "maximum received frames in observe mode; 0 means unlimited")
	fs.Var(&streamFilter, "stream-filter", "display and capture a specific stream ID")
	fs.BoolVar(&cfg.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&cfg.sendGoAway, "send-goaway", cfg.sendGoAway, "send GOAWAY before exit when peer did not")
	fs.StringVar(&cfg.outputFormat, "output", cfg.outputFormat, "output format: text or jsonl")
	fs.StringVar(&cfg.dataFormat, "data-format", cfg.dataFormat, "payload format: text, hex, or both")
	fs.UintVar(&cfg.dataLimit, "data-limit", 0, "truncate payload display to the first N bytes; 0 means unlimited")
	fs.BoolVar(&cfg.decodeHeaders, "decode-headers", cfg.decodeHeaders, "decode HPACK header blocks")
	fs.BoolVar(&cfg.showHeaderBlock, "show-header-block", cfg.showHeaderBlock, "show HPACK/header block fragments")
	fs.StringVar(&cfg.saveOutput, "save-output", "", "write the displayed CLI output to a file as well")
	fs.StringVar(&cfg.saveBody, "save-body", "", "save the captured response body to a file")
	fs.StringVar(&cfg.saveHeaders, "save-headers", "", "save the decoded response headers to a file")
	fs.Var(&cfg.headers, "header", "request header in 'name:value' format; repeatable")
	fs.Var(&cfg.frameFilters, "frame-filter", "only display specific frame types; repeatable")
	fs.Var(&cfg.directionFilters, "direction-filter", "only display sent or received events; repeatable")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg.streamFilter = streamFilter.value
	cfg.hasStreamFilter = streamFilter.set
	if cfg.mode != "request" && cfg.mode != "ping" && cfg.mode != "script" && cfg.mode != "observe" {
		return config{}, fmt.Errorf("invalid mode %q: want request, ping, script, or observe", cfg.mode)
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
