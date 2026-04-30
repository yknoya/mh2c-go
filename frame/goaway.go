package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

type ErrorCode uint32

const (
	ErrNo ErrorCode = 0x0
)

const (
	goAwayLastStreamIDLength = 4
	goAwayErrorCodeLength    = 4
	goAwayMinPayloadLength   = goAwayLastStreamIDLength + goAwayErrorCodeLength
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
	return fmt.Sprintf("GOAWAY last_stream=%d error=0x%08x debug=%d", f.LastStreamID, uint32(f.ErrorCode), len(f.DebugData))
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
		LastStreamID: last & 0x7fff_ffff,
		ErrorCode:    ErrorCode(code),
		DebugData:    append([]byte(nil), payload[goAwayMinPayloadLength:]...),
	}, nil
}
