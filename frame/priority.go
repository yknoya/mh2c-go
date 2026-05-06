package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type PriorityFrame struct {
	FrameHeader Header
	Exclusive   bool
	StreamDep   uint32
	Weight      uint8
}

const (
	priorityDepLength     = 4
	priorityWeightLength  = 1
	priorityPayloadLength = priorityDepLength + priorityWeightLength
)

func NewPriorityFrame(streamID uint32, exclusive bool, streamDep uint32, weight uint8) PriorityFrame {
	frame := PriorityFrame{
		FrameHeader: Header{Type: TypePriority, StreamID: streamID},
		Exclusive:   exclusive,
		StreamDep:   streamDep,
		Weight:      weight,
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f PriorityFrame) Header() Header {
	return f.FrameHeader
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
	return fmt.Sprintf("PRIORITY %s dep=%d exclusive=%t weight=%d", frameHeader(f), f.StreamDep, f.Exclusive, f.Weight)
}

func parsePriorityFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != priorityPayloadLength {
		return nil, fmt.Errorf("PRIORITY payload must be %d bytes, got %d", priorityPayloadLength, len(payload))
	}
	dep, err := wire.ReadUint32(payload[:priorityDepLength])
	if err != nil {
		return nil, err
	}
	return PriorityFrame{
		FrameHeader: header,
		Exclusive:   dep&0x8000_0000 != 0,
		StreamDep:   dep & 0x7fff_ffff,
		Weight:      payload[priorityDepLength],
	}, nil
}
