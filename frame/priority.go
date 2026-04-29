package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type PriorityFrame struct {
	StreamID  uint32
	Exclusive bool
	StreamDep uint32
	Weight    uint8
}

func (f PriorityFrame) Header() Header {
	return Header{Type: TypePriority, StreamID: f.StreamID}
}

func (f PriorityFrame) Payload() []byte {
	dep := f.StreamDep & 0x7fff_ffff
	if f.Exclusive {
		dep |= 0x8000_0000
	}
	payload := wire.AppendUint32(nil, dep)
	return append(payload, f.Weight)
}

func (f PriorityFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f PriorityFrame) String() string {
	return fmt.Sprintf("PRIORITY stream=%d dep=%d exclusive=%t weight=%d", f.StreamID, f.StreamDep, f.Exclusive, f.Weight)
}

func parsePriorityFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != 5 {
		return nil, fmt.Errorf("PRIORITY payload must be 5 bytes, got %d", len(payload))
	}
	dep, err := wire.ReadUint32(payload[:4])
	if err != nil {
		return nil, err
	}
	return PriorityFrame{
		StreamID:  header.StreamID,
		Exclusive: dep&0x8000_0000 != 0,
		StreamDep: dep & 0x7fff_ffff,
		Weight:    payload[4],
	}, nil
}
