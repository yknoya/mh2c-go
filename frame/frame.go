package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type Type uint8

const (
	TypeData         Type = 0x0
	TypeHeaders      Type = 0x1
	TypePriority     Type = 0x2
	TypeRSTStream    Type = 0x3
	TypeSettings     Type = 0x4
	TypePushPromise  Type = 0x5
	TypePing         Type = 0x6
	TypeGoAway       Type = 0x7
	TypeWindowUpdate Type = 0x8
	TypeContinuation Type = 0x9
)

type Header struct {
	Length   uint32
	Type     Type
	Flags    uint8
	StreamID uint32
}

func (h Header) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, wire.FrameHeaderLength)
	var err error
	out, err = wire.AppendUint24(out, h.Length)
	if err != nil {
		return nil, err
	}
	out = append(out, byte(h.Type), h.Flags)
	out = wire.AppendUint32(out, h.StreamID&0x7fff_ffff)
	return out, nil
}

func ParseHeader(src []byte) (Header, error) {
	if len(src) != wire.FrameHeaderLength {
		return Header{}, fmt.Errorf("frame header requires %d bytes, got %d", wire.FrameHeaderLength, len(src))
	}
	length, err := wire.ReadUint24(src[:3])
	if err != nil {
		return Header{}, err
	}
	streamID, err := wire.ReadUint32(src[5:9])
	if err != nil {
		return Header{}, err
	}
	return Header{
		Length:   length,
		Type:     Type(src[3]),
		Flags:    src[4],
		StreamID: streamID & 0x7fff_ffff,
	}, nil
}

type Frame interface {
	Header() Header
	Payload() []byte
	MarshalBinary() ([]byte, error)
}

func Marshal(f Frame) ([]byte, error) {
	return f.MarshalBinary()
}

func Unmarshal(header Header, payload []byte) (Frame, error) {
	if uint32(len(payload)) != header.Length {
		return nil, fmt.Errorf("payload length mismatch: header=%d payload=%d", header.Length, len(payload))
	}
	switch header.Type {
	case TypeData:
		return parseDataFrame(header, payload)
	case TypeHeaders:
		return parseHeadersFrame(header, payload)
	case TypePriority:
		return parsePriorityFrame(header, payload)
	case TypeSettings:
		return parseSettingsFrame(header, payload)
	case TypePushPromise:
		return parsePushPromiseFrame(header, payload)
	case TypePing:
		return parsePingFrame(header, payload)
	case TypeGoAway:
		return parseGoAwayFrame(header, payload)
	case TypeWindowUpdate:
		return parseWindowUpdateFrame(header, payload)
	case TypeContinuation:
		return parseContinuationFrame(header, payload)
	case TypeRSTStream:
		return parseRSTStreamFrame(header, payload)
	default:
		return RawFrameFromParts(header, payload), nil
	}
}

func encode(header Header, payload []byte) ([]byte, error) {
	header.Length = uint32(len(payload))
	head, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(head, payload...), nil
}
