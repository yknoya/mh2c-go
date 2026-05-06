package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type ErrorCode uint32

const (
	ErrNo ErrorCode = 0x0
)

func (c ErrorCode) String() string {
	switch c {
	case ErrNo:
		return "NO_ERROR(0x00000000)"
	default:
		return fmt.Sprintf("0x%08x", uint32(c))
	}
}

const (
	goAwayLastStreamIDLength = 4
	goAwayErrorCodeLength    = 4
	goAwayMinPayloadLength   = goAwayLastStreamIDLength + goAwayErrorCodeLength
)

type GoAwayFrame struct {
	FrameHeader  Header
	LastStreamID uint32
	ErrorCode    ErrorCode
	DebugData    []byte
}

func NewGoAwayFrame(lastStreamID uint32, errorCode ErrorCode, debugData []byte) GoAwayFrame {
	frame := GoAwayFrame{
		FrameHeader:  Header{Type: TypeGoAway, StreamID: 0},
		LastStreamID: lastStreamID,
		ErrorCode:    errorCode,
		DebugData:    append([]byte(nil), debugData...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f GoAwayFrame) Header() Header {
	return f.FrameHeader
}

func (f GoAwayFrame) Payload() []byte {
	payload := make([]byte, 0, goAwayMinPayloadLength+len(f.DebugData))
	last := f.LastStreamID & 0x7fff_ffff
	payload = wire.AppendUint32(payload, last)
	payload = wire.AppendUint32(payload, uint32(f.ErrorCode))
	payload = append(payload, f.DebugData...)
	return payload
}

func (f GoAwayFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f GoAwayFrame) String() string {
	return fmt.Sprintf("GOAWAY %s last_stream=%d error=%s debug=%d", frameHeader(f), f.LastStreamID, f.ErrorCode, len(f.DebugData))
}

func parseGoAwayFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) < goAwayMinPayloadLength {
		return nil, fmt.Errorf("GOAWAY payload must be at least %d bytes", goAwayMinPayloadLength)
	}
	last, err := wire.ReadUint32(payload[:goAwayLastStreamIDLength])
	if err != nil {
		return nil, err
	}
	codeStart := goAwayLastStreamIDLength
	code, err := wire.ReadUint32(payload[codeStart : codeStart+goAwayErrorCodeLength])
	if err != nil {
		return nil, err
	}
	return GoAwayFrame{
		FrameHeader:  header,
		LastStreamID: last & 0x7fff_ffff,
		ErrorCode:    ErrorCode(code),
		DebugData:    append([]byte(nil), payload[goAwayMinPayloadLength:]...),
	}, nil
}
