package frame

import "fmt"

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
	return []byte{
		byte(dep >> 24),
		byte(dep >> 16),
		byte(dep >> 8),
		byte(dep),
		f.Weight,
	}
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
	dep := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	return PriorityFrame{
		StreamID:  header.StreamID,
		Exclusive: dep&0x8000_0000 != 0,
		StreamDep: dep & 0x7fff_ffff,
		Weight:    payload[4],
	}, nil
}
