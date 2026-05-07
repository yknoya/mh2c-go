package frame

import (
	"fmt"

	"github.com/yknoya/mh2c-go/internal/wire"
)

const (
	FlagPushPromiseEndHeaders uint8 = 0x4
	FlagPushPromisePadded     uint8 = 0x8
)

const (
	pushPromisePadLengthFieldLength = 1
	pushPromiseStreamIDLength       = 4
)

type PushPromiseFrame struct {
	FrameHeader      Header
	PadLength        uint8
	PromisedStreamID uint32
	BlockFragment    []byte
}

func NewPushPromiseFrame(streamID uint32, flags uint8, promisedStreamID uint32, block []byte) PushPromiseFrame {
	frame := PushPromiseFrame{
		FrameHeader:      Header{Type: TypePushPromise, StreamID: streamID, Flags: flags},
		PromisedStreamID: promisedStreamID,
		BlockFragment:    append([]byte(nil), block...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f PushPromiseFrame) Header() Header {
	return f.FrameHeader
}

func (f PushPromiseFrame) Payload() []byte {
	flags := f.Header().Flags
	payload := make([]byte, 0, len(f.BlockFragment)+pushPromisePadLengthFieldLength+pushPromiseStreamIDLength)
	if flags&FlagPushPromisePadded != 0 {
		payload = append(payload, f.PadLength)
	}
	promised := f.PromisedStreamID & 0x7fff_ffff
	payload = wire.AppendUint32(payload, promised)
	payload = append(payload, f.BlockFragment...)
	if flags&FlagPushPromisePadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f PushPromiseFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f PushPromiseFrame) String() string {
	return fmt.Sprintf(
		"PUSH_PROMISE %s end_headers=%t promised=%d header_block_fragment_bytes=%d pad=%d",
		frameHeader(f),
		f.Header().Flags&FlagPushPromiseEndHeaders != 0,
		f.PromisedStreamID,
		len(f.BlockFragment),
		f.PadLength,
	)
}

func parsePushPromiseFrame(header Header, payload []byte) (Frame, error) {
	frame := PushPromiseFrame{FrameHeader: header}
	offset := 0
	if header.Flags&FlagPushPromisePadded != 0 {
		if len(payload) == 0 {
			return nil, fmt.Errorf("padded PUSH_PROMISE frame missing pad length")
		}
		frame.PadLength = payload[0]
		offset += pushPromisePadLengthFieldLength
	}
	if len(payload) < offset+pushPromiseStreamIDLength {
		return nil, fmt.Errorf("PUSH_PROMISE frame too short")
	}
	promised, err := wire.ReadUint32(payload[offset : offset+pushPromiseStreamIDLength])
	if err != nil {
		return nil, err
	}
	frame.PromisedStreamID = promised
	frame.PromisedStreamID &= 0x7fff_ffff
	offset += pushPromiseStreamIDLength
	if len(payload) < offset+int(frame.PadLength) {
		return nil, fmt.Errorf("invalid PUSH_PROMISE padding")
	}
	frame.BlockFragment = append([]byte(nil), payload[offset:len(payload)-int(frame.PadLength)]...)
	return frame, nil
}
