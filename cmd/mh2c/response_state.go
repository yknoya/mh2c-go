package main

import (
	"fmt"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

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
