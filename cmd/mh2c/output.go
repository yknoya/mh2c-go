package main

import (
	"fmt"
	"io"

	"github.com/yknoya/mh2c-go/client"
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
	out             io.Writer
	format          string
	dataFormat      string
	dataLimit       uint
	decodeHeaders   bool
	showHeaderBlock bool
	frameFilters    map[string]bool
	directionFilter map[string]bool
	hasStreamFilter bool
	streamFilter    uint32
	capture         *captureManager
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

func (o *outputController) HandleSent(event client.FrameEvent) error {
	headers, warnings := o.sentHeaders(event)
	if !o.shouldDisplay("sent", event.Frame) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("sent", event.Frame, headers, warnings))
	}
	return o.writeTextFrame(">>", event.Frame, headers, warnings)
}

func (o *outputController) HandleReceived(event client.FrameEvent) error {
	if event.DecodeError != nil && (o.decodeHeaders || o.capture != nil) {
		return event.DecodeError
	}
	o.captureReceived(event)
	headers, warnings := o.receivedHeaders(event)
	if !o.shouldDisplay("received", event.Frame) {
		return nil
	}
	if o.format == outputFormatJSONL {
		return o.writeJSON(o.buildJSONEvent("received", event.Frame, headers, warnings))
	}
	return o.writeTextFrame("<<", event.Frame, headers, warnings)
}

func (o *outputController) Flush() error {
	if o.capture == nil {
		return nil
	}
	return o.capture.Flush()
}

func (o *outputController) sentHeaders(event client.FrameEvent) ([]hpack.HeaderField, []string) {
	if !o.decodeHeaders {
		return nil, nil
	}
	warnings := append([]string(nil), event.Warnings...)
	if event.DecodeError != nil {
		warnings = append(warnings, fmt.Sprintf("sent header decode skipped: %v", event.DecodeError))
	}
	return event.Headers, warnings
}

func (o *outputController) receivedHeaders(event client.FrameEvent) ([]hpack.HeaderField, []string) {
	if !o.decodeHeaders {
		return nil, nil
	}
	return event.Headers, event.Warnings
}
