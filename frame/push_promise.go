package frame

import "fmt"

const (
	FlagPushPromiseEndHeaders uint8 = 0x4
	FlagPushPromisePadded     uint8 = 0x8
)

type PushPromiseFrame struct {
	StreamID         uint32
	Flags            uint8
	PadLength        uint8
	PromisedStreamID uint32
	BlockFragment    []byte
}

func (f PushPromiseFrame) Header() Header {
	return Header{Type: TypePushPromise, Flags: f.Flags, StreamID: f.StreamID}
}

func (f PushPromiseFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.BlockFragment)+5)
	if f.Flags&FlagPushPromisePadded != 0 {
		payload = append(payload, f.PadLength)
	}
	promised := f.PromisedStreamID & 0x7fff_ffff
	payload = append(payload, byte(promised>>24), byte(promised>>16), byte(promised>>8), byte(promised))
	payload = append(payload, f.BlockFragment...)
	if f.Flags&FlagPushPromisePadded != 0 {
		payload = append(payload, make([]byte, int(f.PadLength))...)
	}
	return payload
}

func (f PushPromiseFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f PushPromiseFrame) String() string {
	return fmt.Sprintf("PUSH_PROMISE stream=%d promised=%d flags=0x%02x block=%d", f.StreamID, f.PromisedStreamID, f.Flags, len(f.BlockFragment))
}

func parsePushPromiseFrame(header Header, payload []byte) (Frame, error) {
	frame := PushPromiseFrame{StreamID: header.StreamID, Flags: header.Flags}
	offset := 0
	if header.Flags&FlagPushPromisePadded != 0 {
		if len(payload) == 0 {
			return nil, fmt.Errorf("padded PUSH_PROMISE frame missing pad length")
		}
		frame.PadLength = payload[0]
		offset++
	}
	if len(payload) < offset+4 {
		return nil, fmt.Errorf("PUSH_PROMISE frame too short")
	}
	frame.PromisedStreamID = uint32(payload[offset])<<24 | uint32(payload[offset+1])<<16 | uint32(payload[offset+2])<<8 | uint32(payload[offset+3])
	frame.PromisedStreamID &= 0x7fff_ffff
	offset += 4
	if len(payload) < offset+int(frame.PadLength) {
		return nil, fmt.Errorf("invalid PUSH_PROMISE padding")
	}
	frame.BlockFragment = append([]byte(nil), payload[offset:len(payload)-int(frame.PadLength)]...)
	return frame, nil
}
