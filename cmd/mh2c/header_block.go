package main

import (
	"fmt"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func consumeHeaderBlockForDisplay(
	pendingStream *uint32,
	pendingBlock *[]byte,
	pendingEnd *bool,
	received frame.Frame,
	decode func([]byte) (hpack.DecodeReport, error),
) ([]hpack.HeaderField, []string, uint32, bool, error) {
	switch typed := received.(type) {
	case frame.HeadersFrame:
		if typed.Flags&frame.FlagHeadersEndHeaders != 0 {
			report, err := decode(typed.BlockFragment)
			return report.Fields, report.Warnings, typed.StreamID, typed.Flags&frame.FlagHeadersEndStream != 0, err
		}
		*pendingStream = typed.StreamID
		*pendingBlock = append([]byte(nil), typed.BlockFragment...)
		*pendingEnd = typed.Flags&frame.FlagHeadersEndStream != 0
	case frame.ContinuationFrame:
		if *pendingStream == 0 {
			return nil, nil, 0, false, fmt.Errorf("unexpected CONTINUATION frame on stream %d", typed.StreamID)
		}
		if typed.StreamID != *pendingStream {
			return nil, nil, 0, false, fmt.Errorf("CONTINUATION stream mismatch: got %d, want %d", typed.StreamID, *pendingStream)
		}
		*pendingBlock = append(*pendingBlock, typed.BlockFragment...)
		if typed.Flags&frame.FlagContinuationEndHeaders != 0 {
			report, err := decode(*pendingBlock)
			streamID := *pendingStream
			endStream := *pendingEnd
			*pendingStream = 0
			*pendingBlock = nil
			*pendingEnd = false
			return report.Fields, report.Warnings, streamID, endStream, err
		}
	}
	return nil, nil, 0, false, nil
}
