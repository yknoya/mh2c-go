package main

import (
	"fmt"
	"io"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

const (
	outputFormatText  = "text"
	outputFormatJSONL = "jsonl"

	dataFormatText = "text"
	dataFormatHex  = "hex"
	dataFormatBoth = "both"
)

type outputController struct {
	out               io.Writer
	format            string
	dataFormat        string
	dataLimit         uint
	decodeHeaders     bool
	showHeaderBlock   bool
	frameFilters      map[string]bool
	directionFilter   map[string]bool
	hasStreamFilter   bool
	streamFilter      uint32
	sentPendingStream uint32
	sentPendingBlock  []byte
	sentPendingEnd    bool
	pendingStream     uint32
	pendingBlock      []byte
	pendingEnd        bool
	capture           *captureManager
}

func newOutputController(out io.Writer, cfg config) (*outputController, error) {
	filters, err := buildFrameFilterSet(cfg.frameFilters)
	if err != nil {
		return nil, err
	}

	controller := &outputController{
		out:             out,
		format:          cfg.outputFormat,
		dataFormat:      cfg.dataFormat,
		dataLimit:       cfg.dataLimit,
		decodeHeaders:   cfg.decodeHeaders,
		showHeaderBlock: cfg.showHeaderBlock,
		frameFilters:    filters,
		directionFilter: buildDirectionFilterSet(cfg.directionFilters),
		hasStreamFilter: cfg.hasStreamFilter,
		streamFilter:    uint32(cfg.streamFilter),
	}

	switch cfg.mode {
	case "request":
		if cfg.saveBody != "" || cfg.saveHeaders != "" {
			targetStream := uint32(cfg.streamID)
			if cfg.hasStreamFilter {
				targetStream = uint32(cfg.streamFilter)
			}
			controller.capture = newExplicitCaptureManager(targetStream, cfg.saveBody, cfg.saveHeaders)
		}
	case "observe":
		if cfg.saveBody != "" || cfg.saveHeaders != "" {
			if cfg.hasStreamFilter {
				controller.capture = newExplicitCaptureManager(uint32(cfg.streamFilter), cfg.saveBody, cfg.saveHeaders)
			} else {
				controller.capture = newAutoCaptureManager(cfg.saveBody, cfg.saveHeaders)
			}
		}
	}

	return controller, nil
}

func (o *outputController) PrintNotice(direction, kind, summary string) error {
	if !o.shouldDisplayDirection(direction) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(jsonFrameEvent{
			Direction: direction,
			FrameType: kind,
			Summary:   summary,
		})
	}
	prefix := "<<"
	if direction == "sent" {
		prefix = ">>"
	}
	_, err := fmt.Fprintf(o.out, "%s %s\n", prefix, summary)
	return err
}

func (o *outputController) HandleSent(h2c *client.Client, f frame.Frame) error {
	headers, warnings := o.decodeSentHeaders(h2c, f)
	if !o.shouldDisplay("sent", f) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("sent", f, headers, warnings))
	}
	return o.writeTextFrame(">>", f, headers, warnings)
}

func (o *outputController) HandleReceived(h2c *client.Client, f frame.Frame) error {
	headers, warnings, streamID, endStream, err := o.decodeReceivedHeaders(h2c, f)
	if err != nil {
		return err
	}
	o.captureReceived(streamID, headers, endStream, f)
	if !o.shouldDisplay("received", f) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("received", f, headers, warnings))
	}
	return o.writeTextFrame("<<", f, headers, warnings)
}

func (o *outputController) Flush() error {
	if o.capture == nil {
		return nil
	}
	return o.capture.Flush()
}

func (o *outputController) decodeSentHeaders(h2c *client.Client, f frame.Frame) ([]hpack.HeaderField, []string) {
	if !o.decodeHeaders || h2c == nil {
		return nil, nil
	}

	headers, warnings, _, _, err := consumeHeaderBlockForDisplay(
		&o.sentPendingStream,
		&o.sentPendingBlock,
		&o.sentPendingEnd,
		f,
		h2c.RequestCodec().DecodeDetailed,
	)
	if err != nil {
		o.sentPendingStream = 0
		o.sentPendingBlock = nil
		o.sentPendingEnd = false
		return nil, []string{fmt.Sprintf("sent header decode skipped: %v", err)}
	}
	if len(headers) > 0 {
		return headers, warnings
	}

	typed, ok := f.(frame.PushPromiseFrame)
	if !ok || typed.Header().Flags&frame.FlagPushPromiseEndHeaders == 0 {
		return nil, nil
	}
	report, err := h2c.RequestCodec().DecodeDetailed(typed.BlockFragment)
	if err != nil {
		return nil, []string{fmt.Sprintf("sent header decode skipped: %v", err)}
	}
	return report.Fields, report.Warnings
}

func (o *outputController) decodeReceivedHeaders(h2c *client.Client, f frame.Frame) ([]hpack.HeaderField, []string, uint32, bool, error) {
	needTrackedDecode := o.decodeHeaders || o.capture != nil
	if needTrackedDecode {
		headers, warnings, streamID, endStream, err := consumeHeaderBlockForDisplay(
			&o.pendingStream,
			&o.pendingBlock,
			&o.pendingEnd,
			f,
			h2c.DecodeHeadersDetailed,
		)
		if err != nil {
			return nil, nil, 0, false, err
		}
		if len(headers) > 0 {
			return headers, warnings, streamID, endStream, nil
		}
	}

	if !o.decodeHeaders {
		return nil, nil, 0, false, nil
	}

	typed, ok := f.(frame.PushPromiseFrame)
	if !ok || typed.Header().Flags&frame.FlagPushPromiseEndHeaders == 0 {
		return nil, nil, 0, false, nil
	}
	report, err := h2c.DecodeHeadersDetailed(typed.BlockFragment)
	if err != nil {
		return nil, nil, 0, false, err
	}
	return report.Fields, report.Warnings, typed.Header().StreamID, false, nil
}
