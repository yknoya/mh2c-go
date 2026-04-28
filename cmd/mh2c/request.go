package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

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

func sendRequest(h2c *client.Client, streamID uint32, fields []hpack.HeaderField, body []byte, out *outputController) error {
	block, err := h2c.EncodeHeaders(fields)
	if err != nil {
		return err
	}

	flags := uint8(frame.FlagHeadersEndHeaders)
	if len(body) == 0 {
		flags |= frame.FlagHeadersEndStream
	}
	headers := frame.HeadersFrame{
		StreamID:      streamID,
		Flags:         flags,
		BlockFragment: block,
	}
	if err := sendFrameAndReport(h2c, out, headers); err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return sendFrameAndReport(h2c, out, frame.DataFrame{
		StreamID: streamID,
		Flags:    frame.FlagDataEndStream,
		Data:     body,
	})
}
