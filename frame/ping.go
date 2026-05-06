package frame

import "fmt"

const FlagPingAck uint8 = 0x1

const pingPayloadLength = 8

type PingFrame struct {
	FrameHeader Header
	Data        [pingPayloadLength]byte
}

func NewPingFrame(flags uint8, data [pingPayloadLength]byte) PingFrame {
	frame := PingFrame{
		FrameHeader: Header{Type: TypePing, Flags: flags, StreamID: 0},
		Data:        data,
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f PingFrame) Header() Header {
	return f.FrameHeader
}

func (f PingFrame) Payload() []byte {
	return append([]byte(nil), f.Data[:]...)
}

func (f PingFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f PingFrame) String() string {
	return fmt.Sprintf("PING %s ack=%t data=%x", frameHeader(f), f.Header().Flags&FlagPingAck != 0, f.Data)
}

func parsePingFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != pingPayloadLength {
		return nil, fmt.Errorf("PING payload must be %d bytes", pingPayloadLength)
	}
	var data [pingPayloadLength]byte
	copy(data[:], payload)
	return PingFrame{FrameHeader: header, Data: data}, nil
}
