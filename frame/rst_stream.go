package frame

import "fmt"

type RSTStreamFrame struct {
	StreamID  uint32
	ErrorCode ErrorCode
}

func (f RSTStreamFrame) Header() Header {
	return Header{Type: TypeRSTStream, StreamID: f.StreamID}
}

func (f RSTStreamFrame) Payload() []byte {
	code := uint32(f.ErrorCode)
	return []byte{byte(code >> 24), byte(code >> 16), byte(code >> 8), byte(code)}
}

func (f RSTStreamFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f RSTStreamFrame) String() string {
	return fmt.Sprintf("RST_STREAM stream=%d error=0x%08x", f.StreamID, uint32(f.ErrorCode))
}

func parseRSTStreamFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != 4 {
		return nil, fmt.Errorf("RST_STREAM payload must be 4 bytes")
	}
	code := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	return RSTStreamFrame{StreamID: header.StreamID, ErrorCode: ErrorCode(code)}, nil
}
