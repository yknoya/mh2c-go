package frame

import "fmt"

type RawFrame struct {
	header  Header
	payload []byte
}

func RawFrameFromParts(header Header, payload []byte) RawFrame {
	copied := append([]byte(nil), payload...)
	header.Length = uint32(len(copied))
	return RawFrame{header: header, payload: copied}
}

func RawFrameFromExactParts(header Header, payload []byte) RawFrame {
	return RawFrame{header: header, payload: append([]byte(nil), payload...)}
}

func (f RawFrame) Header() Header {
	return f.header
}

func (f RawFrame) Payload() []byte {
	return append([]byte(nil), f.payload...)
}

func (f RawFrame) MarshalBinary() ([]byte, error) {
	head, err := f.header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(head, f.payload...), nil
}

func (f RawFrame) String() string {
	return fmt.Sprintf("RAW %s payload=%d", f.header, len(f.payload))
}
