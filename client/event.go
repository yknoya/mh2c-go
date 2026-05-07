package client

import (
	"fmt"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

// FrameEvent is a frame plus any header fields decoded while tracking it.
type FrameEvent struct {
	Frame               frame.Frame
	Headers             []hpack.HeaderField
	Warnings            []string
	StreamID            uint32
	EndStream           bool
	HeaderBlockComplete bool
	DecodeError         error
}

type headerBlockTracker struct {
	pendingStream uint32
	pendingBlock  []byte
	pendingEnd    bool
}

// TrackSentFrame updates client-side HPACK observation state for a sent frame.
func (c *Client) TrackSentFrame(f frame.Frame) FrameEvent {
	return c.sentHeaderBlock.consume(f, c.requestCodec.DecodeDetailed)
}

// TrackReceivedFrame updates client-side HPACK observation state for a received frame.
func (c *Client) TrackReceivedFrame(f frame.Frame) FrameEvent {
	return c.receivedHeaderBlock.consume(f, c.responseCodec.DecodeDetailed)
}

func (t *headerBlockTracker) consume(f frame.Frame, decode func([]byte) (hpack.DecodeReport, error)) FrameEvent {
	event := FrameEvent{Frame: f}
	switch typed := f.(type) {
	case frame.HeadersFrame:
		return t.consumeHeaderStart(event, typed.Header().StreamID, typed.Header().Flags&frame.FlagHeadersEndStream != 0, typed.Header().Flags&frame.FlagHeadersEndHeaders != 0, typed.BlockFragment, decode)
	case frame.PushPromiseFrame:
		return t.consumeHeaderStart(event, typed.Header().StreamID, false, typed.Header().Flags&frame.FlagPushPromiseEndHeaders != 0, typed.BlockFragment, decode)
	case frame.ContinuationFrame:
		if t.pendingStream == 0 {
			event.DecodeError = fmt.Errorf("unexpected CONTINUATION frame on stream %d", typed.Header().StreamID)
			return event
		}
		if typed.Header().StreamID != t.pendingStream {
			event.DecodeError = fmt.Errorf("CONTINUATION stream mismatch: got %d, want %d", typed.Header().StreamID, t.pendingStream)
			t.reset()
			return event
		}
		t.pendingBlock = append(t.pendingBlock, typed.BlockFragment...)
		if typed.Header().Flags&frame.FlagContinuationEndHeaders != 0 {
			return t.finish(event, t.pendingStream, t.pendingEnd, t.pendingBlock, decode)
		}
	}
	return event
}

func (t *headerBlockTracker) consumeHeaderStart(event FrameEvent, streamID uint32, endStream, endHeaders bool, block []byte, decode func([]byte) (hpack.DecodeReport, error)) FrameEvent {
	if t.pendingStream != 0 {
		event.DecodeError = fmt.Errorf("received header block before previous header block finished on stream %d", t.pendingStream)
		t.reset()
		return event
	}
	if endHeaders {
		return t.finish(event, streamID, endStream, block, decode)
	}
	t.pendingStream = streamID
	t.pendingBlock = append([]byte(nil), block...)
	t.pendingEnd = endStream
	return event
}

func (t *headerBlockTracker) finish(event FrameEvent, streamID uint32, endStream bool, block []byte, decode func([]byte) (hpack.DecodeReport, error)) FrameEvent {
	event.HeaderBlockComplete = true
	event.StreamID = streamID
	event.EndStream = endStream
	report, err := decode(block)
	if err != nil {
		event.DecodeError = err
		t.reset()
		return event
	}
	event.Headers = report.Fields
	event.Warnings = report.Warnings
	t.reset()
	return event
}

func (t *headerBlockTracker) reset() {
	t.pendingStream = 0
	t.pendingBlock = nil
	t.pendingEnd = false
}
