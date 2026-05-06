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
	FrameHeader   Header
	PadLength     uint8
	Priority      *PriorityParam
	BlockFragment []byte
}

func NewHeadersFrame(streamID uint32, flags uint8, block []byte) HeadersFrame {
	frame := HeadersFrame{
		FrameHeader:   Header{Type: TypeHeaders, StreamID: streamID, Flags: flags},
		BlockFragment: append([]byte(nil), block...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f HeadersFrame) Header() Header {
	return f.FrameHeader
}

func (f HeadersFrame) Payload() []byte {
	flags := f.Header().Flags
	payload := make([]byte, 0, len(f.BlockFragment)+headersPadLengthFieldLength+headersPriorityParamLength)
	if flags&FlagHeadersPadded != 0 {
		payload = append(payload, f.PadLength)
	}
	if flags&FlagHeadersPriority != 0 {
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
	if flags&FlagHeadersPadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f HeadersFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f HeadersFrame) String() string {
	priority := "none"
	if f.Priority != nil {
		priority = fmt.Sprintf("dep=%d exclusive=%t weight=%d", f.Priority.StreamDep, f.Priority.Exclusive, f.Priority.Weight)
	} else if f.Header().Flags&FlagHeadersPriority != 0 {
		priority = "present=<nil>"
	}
	return fmt.Sprintf(
		"HEADERS %s end_stream=%t end_headers=%t block=%d pad=%d priority=%s",
		frameHeader(f),
		f.Header().Flags&FlagHeadersEndStream != 0,
		f.Header().Flags&FlagHeadersEndHeaders != 0,
		len(f.BlockFragment),
		f.PadLength,
		priority,
	)
}

func parseHeadersFrame(header Header, payload []byte) (Frame, error) {
	frame := HeadersFrame{FrameHeader: header}
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
