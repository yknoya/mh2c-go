package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type WindowUpdateFrame struct {
	StreamID  uint32
	Increment uint32
}

const windowUpdatePayloadLength = 4

func (f WindowUpdateFrame) Header() Header {
	return Header{Type: TypeWindowUpdate, StreamID: f.StreamID}
}

func (f WindowUpdateFrame) Payload() []byte {
	incr := f.Increment & 0x7fff_ffff
	return wire.AppendUint32(nil, incr)
}

func (f WindowUpdateFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f WindowUpdateFrame) String() string {
	return fmt.Sprintf("WINDOW_UPDATE stream=%d increment=%d", f.StreamID, f.Increment)
}

func parseWindowUpdateFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != windowUpdatePayloadLength {
		return nil, fmt.Errorf("WINDOW_UPDATE payload must be %d bytes", windowUpdatePayloadLength)
	}
	incr, err := wire.ReadUint32(payload)
	if err != nil {
		return nil, err
	}
	return WindowUpdateFrame{StreamID: header.StreamID, Increment: incr & 0x7fff_ffff}, nil
}
