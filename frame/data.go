package frame

import "fmt"

const (
	FlagDataEndStream uint8 = 0x1
	FlagDataPadded    uint8 = 0x8
)

const dataPadLengthFieldLength = 1

type DataFrame struct {
	StreamID  uint32
	Flags     uint8
	PadLength uint8
	Data      []byte
}

func (f DataFrame) Header() Header {
	return Header{Type: TypeData, Flags: f.Flags, StreamID: f.StreamID}
}

func (f DataFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.Data)+dataPadLengthFieldLength)
	if f.Flags&FlagDataPadded != 0 {
		payload = append(payload, f.PadLength)
	}
	payload = append(payload, f.Data...)
	if f.Flags&FlagDataPadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f DataFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f DataFrame) String() string {
	return fmt.Sprintf("DATA stream=%d flags=0x%02x len=%d", f.StreamID, f.Flags, len(f.Data))
}

func parseDataFrame(header Header, payload []byte) (Frame, error) {
	frame := DataFrame{StreamID: header.StreamID, Flags: header.Flags}
	offset := 0
	if header.Flags&FlagDataPadded != 0 {
		if len(payload) == 0 {
			return nil, fmt.Errorf("padded DATA frame missing pad length")
		}
		frame.PadLength = payload[0]
		offset += dataPadLengthFieldLength
	}
	if len(payload) < offset+int(frame.PadLength) {
		return nil, fmt.Errorf("invalid DATA padding")
	}
	frame.Data = append([]byte(nil), payload[offset:len(payload)-int(frame.PadLength)]...)
	return frame, nil
}
