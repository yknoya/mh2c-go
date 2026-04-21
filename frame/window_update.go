package frame

import "fmt"

type WindowUpdateFrame struct {
	StreamID  uint32
	Increment uint32
}

func (f WindowUpdateFrame) Header() Header {
	return Header{Type: TypeWindowUpdate, StreamID: f.StreamID}
}

func (f WindowUpdateFrame) Payload() []byte {
	incr := f.Increment & 0x7fff_ffff
	return []byte{byte(incr >> 24), byte(incr >> 16), byte(incr >> 8), byte(incr)}
}

func (f WindowUpdateFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f WindowUpdateFrame) String() string {
	return fmt.Sprintf("WINDOW_UPDATE stream=%d increment=%d", f.StreamID, f.Increment)
}

func parseWindowUpdateFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != 4 {
		return nil, fmt.Errorf("WINDOW_UPDATE payload must be 4 bytes")
	}
	incr := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	return WindowUpdateFrame{StreamID: header.StreamID, Increment: incr & 0x7fff_ffff}, nil
}
