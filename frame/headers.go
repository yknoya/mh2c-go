package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

const (
	FlagHeadersEndStream  uint8 = 0x1
	FlagHeadersEndHeaders uint8 = 0x4
	FlagHeadersPadded     uint8 = 0x8
	FlagHeadersPriority   uint8 = 0x20
)

const (
	headersPadLengthFieldLength = 1
	headersPriorityDepLength    = 4
	headersPriorityWeightLength = 1
	headersPriorityParamLength  = headersPriorityDepLength + headersPriorityWeightLength
)

type PriorityParam struct {
	Exclusive bool
	StreamDep uint32
	Weight    uint8
}

type HeadersFrame struct {
	StreamID      uint32
	Flags         uint8
	PadLength     uint8
	Priority      *PriorityParam
	BlockFragment []byte
}

func (f HeadersFrame) Header() Header {
	return Header{Type: TypeHeaders, Flags: f.Flags, StreamID: f.StreamID}
}

func (f HeadersFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.BlockFragment)+headersPadLengthFieldLength+headersPriorityParamLength)
	if f.Flags&FlagHeadersPadded != 0 {
		payload = append(payload, f.PadLength)
	}
	if f.Flags&FlagHeadersPriority != 0 {
		var dep uint32
		if f.Priority != nil {
			dep = f.Priority.StreamDep & 0x7fff_ffff
			if f.Priority.Exclusive {
				dep |= 0x8000_0000
			}
		}
		payload = wire.AppendUint32(payload, dep)
		if f.Priority != nil {
			payload = append(payload, f.Priority.Weight)
		} else {
			payload = append(payload, 0)
		}
	}
	payload = append(payload, f.BlockFragment...)
	if f.Flags&FlagHeadersPadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f HeadersFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f HeadersFrame) String() string {
	return fmt.Sprintf("HEADERS stream=%d flags=0x%02x block=%d", f.StreamID, f.Flags, len(f.BlockFragment))
}

func parseHeadersFrame(header Header, payload []byte) (Frame, error) {
	frame := HeadersFrame{StreamID: header.StreamID, Flags: header.Flags}
	offset := 0
	if header.Flags&FlagHeadersPadded != 0 {
		if len(payload) == 0 {
			return nil, fmt.Errorf("padded HEADERS frame missing pad length")
		}
		frame.PadLength = payload[0]
		offset += headersPadLengthFieldLength
	}
	if header.Flags&FlagHeadersPriority != 0 {
		if len(payload) < offset+headersPriorityParamLength {
			return nil, fmt.Errorf("priority HEADERS frame too short")
		}
		dep, err := wire.ReadUint32(payload[offset : offset+headersPriorityDepLength])
		if err != nil {
			return nil, err
		}
		frame.Priority = &PriorityParam{
			Exclusive: dep&0x8000_0000 != 0,
			StreamDep: dep & 0x7fff_ffff,
			Weight:    payload[offset+headersPriorityDepLength],
		}
		offset += headersPriorityParamLength
	}
	if len(payload) < offset+int(frame.PadLength) {
		return nil, fmt.Errorf("invalid HEADERS padding")
	}
	frame.BlockFragment = append([]byte(nil), payload[offset:len(payload)-int(frame.PadLength)]...)
	return frame, nil
}
