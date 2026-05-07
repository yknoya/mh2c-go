package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

type captureManager struct {
	bodyPath       string
	headersPath    string
	explicitStream bool
	targetStreamID uint32
	selectedStream uint32
	streams        map[uint32]*capturedStream
}

type capturedStream struct {
	headers []hpack.HeaderField
	body    bytes.Buffer
	done    bool
}

func newExplicitCaptureManager(streamID uint32, bodyPath, headersPath string) *captureManager {
	return &captureManager{
		bodyPath:       bodyPath,
		headersPath:    headersPath,
		explicitStream: true,
		targetStreamID: streamID,
		streams:        map[uint32]*capturedStream{},
	}
}

func newAutoCaptureManager(bodyPath, headersPath string) *captureManager {
	return &captureManager{
		bodyPath:    bodyPath,
		headersPath: headersPath,
		streams:     map[uint32]*capturedStream{},
	}
}

func (o *outputController) captureReceived(event client.FrameEvent) {
	if o.capture == nil {
		return
	}

	if event.HeaderBlockComplete && len(event.Headers) > 0 {
		o.capture.RecordHeaders(event.StreamID, event.Headers, event.EndStream)
	}

	if typed, ok := event.Frame.(frame.DataFrame); ok {
		o.capture.RecordData(typed.Header().StreamID, typed.Data, typed.Header().Flags&frame.FlagDataEndStream != 0)
	}
}

func (m *captureManager) RecordHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) {
	stream := m.stream(streamID)
	stream.headers = append(stream.headers, headers...)
	if endStream {
		stream.done = true
		m.selectStream(streamID)
	}
}

func (m *captureManager) RecordData(streamID uint32, data []byte, endStream bool) {
	stream := m.stream(streamID)
	_, _ = stream.body.Write(data)
	if endStream {
		stream.done = true
		m.selectStream(streamID)
	}
}

func (m *captureManager) Flush() error {
	stream, ok := m.selected()
	if !ok {
		return nil
	}
	if m.bodyPath != "" {
		if err := os.WriteFile(m.bodyPath, stream.body.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if m.headersPath != "" {
		var body strings.Builder
		for _, field := range stream.headers {
			if _, err := fmt.Fprintf(&body, "%s: %s\n", field.Name, field.Value); err != nil {
				return err
			}
		}
		if err := os.WriteFile(m.headersPath, []byte(body.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *captureManager) stream(streamID uint32) *capturedStream {
	stream, ok := m.streams[streamID]
	if !ok {
		stream = &capturedStream{}
		m.streams[streamID] = stream
	}
	return stream
}

func (m *captureManager) selectStream(streamID uint32) {
	if m.explicitStream {
		if streamID == m.targetStreamID {
			m.selectedStream = streamID
		}
		return
	}
	if m.selectedStream == 0 {
		m.selectedStream = streamID
	}
}

func (m *captureManager) selected() (*capturedStream, bool) {
	if m.explicitStream {
		stream, ok := m.streams[m.targetStreamID]
		return stream, ok
	}
	if m.selectedStream == 0 {
		return nil, false
	}
	stream, ok := m.streams[m.selectedStream]
	return stream, ok
}
