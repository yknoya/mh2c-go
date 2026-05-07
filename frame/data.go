package frame

import "fmt"

const (
	FlagDataEndStream uint8 = 0x1
	FlagDataPadded    uint8 = 0x8
)

const dataPadLengthFieldLength = 1

type DataFrame struct {
	FrameHeader Header
	PadLength   uint8
	Data        []byte
}

// NewDataFrame builds a DATA frame from the ordinary stream metadata and data bytes.
func NewDataFrame(streamID uint32, flags uint8, data []byte) DataFrame {
	frame := DataFrame{
		FrameHeader: Header{Type: TypeData, StreamID: streamID, Flags: flags},
		Data:        append([]byte(nil), data...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f DataFrame) Header() Header {
	return f.FrameHeader
}

func (f DataFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.Data)+dataPadLengthFieldLength)
	if f.Header().Flags&FlagDataPadded != 0 {
		payload = append(payload, f.PadLength)
	}
	payload = append(payload, f.Data...)
	if f.Header().Flags&FlagDataPadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f DataFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f DataFrame) String() string {
	return fmt.Sprintf("DATA %s end_stream=%t data_bytes=%d pad=%d", frameHeader(f), f.Header().Flags&FlagDataEndStream != 0, len(f.Data), f.PadLength)
}

func parseDataFrame(header Header, payload []byte) (Frame, error) {
	frame := DataFrame{FrameHeader: header}
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
