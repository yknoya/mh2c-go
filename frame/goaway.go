package frame

import "fmt"

type ErrorCode uint32

const (
	ErrNo ErrorCode = 0x0
)

type GoAwayFrame struct {
	LastStreamID uint32
	ErrorCode    ErrorCode
	DebugData    []byte
}

func (f GoAwayFrame) Header() Header {
	return Header{Type: TypeGoAway, StreamID: 0}
}

func (f GoAwayFrame) Payload() []byte {
	payload := make([]byte, 0, 8+len(f.DebugData))
	last := f.LastStreamID & 0x7fff_ffff
	payload = append(payload, byte(last>>24), byte(last>>16), byte(last>>8), byte(last))
	code := uint32(f.ErrorCode)
	payload = append(payload, byte(code>>24), byte(code>>16), byte(code>>8), byte(code))
	payload = append(payload, f.DebugData...)
	return payload
}

func (f GoAwayFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f GoAwayFrame) String() string {
	return fmt.Sprintf("GOAWAY last_stream=%d error=0x%08x debug=%d", f.LastStreamID, uint32(f.ErrorCode), len(f.DebugData))
}

func parseGoAwayFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("GOAWAY payload must be at least 8 bytes")
	}
	last := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	code := uint32(payload[4])<<24 | uint32(payload[5])<<16 | uint32(payload[6])<<8 | uint32(payload[7])
	return GoAwayFrame{
		LastStreamID: last & 0x7fff_ffff,
		ErrorCode:    ErrorCode(code),
		DebugData:    append([]byte(nil), payload[8:]...),
	}, nil
}
