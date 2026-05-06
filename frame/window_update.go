package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type WindowUpdateFrame struct {
	FrameHeader Header
	Increment   uint32
}

const windowUpdatePayloadLength = 4

func NewWindowUpdateFrame(streamID uint32, increment uint32) WindowUpdateFrame {
	frame := WindowUpdateFrame{
		FrameHeader: Header{Type: TypeWindowUpdate, StreamID: streamID},
		Increment:   increment,
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f WindowUpdateFrame) Header() Header {
	return f.FrameHeader
}

func (f WindowUpdateFrame) Payload() []byte {
	incr := f.Increment & 0x7fff_ffff
	return wire.AppendUint32(nil, incr)
}

func (f WindowUpdateFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f WindowUpdateFrame) String() string {
	return fmt.Sprintf("WINDOW_UPDATE %s increment=%d", frameHeader(f), f.Increment)
}

func parseWindowUpdateFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != windowUpdatePayloadLength {
		return nil, fmt.Errorf("WINDOW_UPDATE payload must be %d bytes", windowUpdatePayloadLength)
	}
	incr, err := wire.ReadUint32(payload)
	if err != nil {
		return nil, err
	}
	return WindowUpdateFrame{FrameHeader: header, Increment: incr & 0x7fff_ffff}, nil
}
