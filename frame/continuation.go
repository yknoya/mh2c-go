package frame

import "fmt"

const FlagContinuationEndHeaders uint8 = 0x4

type ContinuationFrame struct {
	StreamID      uint32
	Flags         uint8
	BlockFragment []byte
}

func (f ContinuationFrame) Header() Header {
	return Header{Type: TypeContinuation, Flags: f.Flags, StreamID: f.StreamID}
}

func (f ContinuationFrame) Payload() []byte {
	return append([]byte(nil), f.BlockFragment...)
}

func (f ContinuationFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f ContinuationFrame) String() string {
	return fmt.Sprintf("CONTINUATION %s end_headers=%t block=%d", frameHeader(f), f.Flags&FlagContinuationEndHeaders != 0, len(f.BlockFragment))
}

func parseContinuationFrame(header Header, payload []byte) (Frame, error) {
	return ContinuationFrame{
		StreamID:      header.StreamID,
		Flags:         header.Flags,
		BlockFragment: append([]byte(nil), payload...),
	}, nil
}
