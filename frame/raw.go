package frame

type RawFrame struct {
	header  Header
	payload []byte
}

func RawFrameFromParts(header Header, payload []byte) RawFrame {
	copied := append([]byte(nil), payload...)
	header.Length = uint32(len(copied))
	return RawFrame{header: header, payload: copied}
}

func (f RawFrame) Header() Header {
	return f.header
}

func (f RawFrame) Payload() []byte {
	return append([]byte(nil), f.payload...)
}

func (f RawFrame) MarshalBinary() ([]byte, error) {
	return encode(f.header, f.payload)
}
