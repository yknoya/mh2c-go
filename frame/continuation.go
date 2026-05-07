package frame

import "fmt"

const FlagContinuationEndHeaders uint8 = 0x4

type ContinuationFrame struct {
	FrameHeader   Header
	BlockFragment []byte
}

func NewContinuationFrame(streamID uint32, flags uint8, block []byte) ContinuationFrame {
	frame := ContinuationFrame{
		FrameHeader:   Header{Type: TypeContinuation, StreamID: streamID, Flags: flags},
		BlockFragment: append([]byte(nil), block...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f ContinuationFrame) Header() Header {
	return f.FrameHeader
}

func (f ContinuationFrame) Payload() []byte {
	return append([]byte(nil), f.BlockFragment...)
}

func (f ContinuationFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f ContinuationFrame) String() string {
	return fmt.Sprintf("CONTINUATION %s end_headers=%t header_block_fragment_bytes=%d", frameHeader(f), f.Header().Flags&FlagContinuationEndHeaders != 0, len(f.BlockFragment))
}

func parseContinuationFrame(header Header, payload []byte) (Frame, error) {
	return ContinuationFrame{
		FrameHeader:   header,
		BlockFragment: append([]byte(nil), payload...),
	}, nil
}
