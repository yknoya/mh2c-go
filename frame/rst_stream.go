package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type RSTStreamFrame struct {
	StreamID  uint32
	ErrorCode ErrorCode
}

const rstStreamPayloadLength = 4

func (f RSTStreamFrame) Header() Header {
	return Header{Type: TypeRSTStream, StreamID: f.StreamID}
}

func (f RSTStreamFrame) Payload() []byte {
	return wire.AppendUint32(nil, uint32(f.ErrorCode))
}

func (f RSTStreamFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f RSTStreamFrame) String() string {
	return fmt.Sprintf("RST_STREAM stream=%d error=0x%08x", f.StreamID, uint32(f.ErrorCode))
}

func parseRSTStreamFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != rstStreamPayloadLength {
		return nil, fmt.Errorf("RST_STREAM payload must be %d bytes", rstStreamPayloadLength)
	}
	code, err := wire.ReadUint32(payload)
	if err != nil {
		return nil, err
	}
	return RSTStreamFrame{StreamID: header.StreamID, ErrorCode: ErrorCode(code)}, nil
}
