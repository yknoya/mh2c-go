package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type RSTStreamFrame struct {
	FrameHeader Header
	ErrorCode   ErrorCode
}

const rstStreamPayloadLength = 4

func NewRSTStreamFrame(streamID uint32, errorCode ErrorCode) RSTStreamFrame {
	frame := RSTStreamFrame{
		FrameHeader: Header{Type: TypeRSTStream, StreamID: streamID},
		ErrorCode:   errorCode,
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f RSTStreamFrame) Header() Header {
	return f.FrameHeader
}

func (f RSTStreamFrame) Payload() []byte {
	return wire.AppendUint32(nil, uint32(f.ErrorCode))
}

func (f RSTStreamFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f RSTStreamFrame) String() string {
	return fmt.Sprintf("RST_STREAM %s error=%s", frameHeader(f), f.ErrorCode)
}

func parseRSTStreamFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != rstStreamPayloadLength {
		return nil, fmt.Errorf("RST_STREAM payload must be %d bytes", rstStreamPayloadLength)
	}
	code, err := wire.ReadUint32(payload)
	if err != nil {
		return nil, err
	}
	return RSTStreamFrame{FrameHeader: header, ErrorCode: ErrorCode(code)}, nil
}
