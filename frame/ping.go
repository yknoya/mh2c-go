package frame

import "fmt"

const FlagPingAck uint8 = 0x1

const pingPayloadLength = 8

type PingFrame struct {
	Flags uint8
	Data  [pingPayloadLength]byte
}

func (f PingFrame) Header() Header {
	return Header{Type: TypePing, Flags: f.Flags, StreamID: 0}
}

func (f PingFrame) Payload() []byte {
	return append([]byte(nil), f.Data[:]...)
}

func (f PingFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f PingFrame) String() string {
	return fmt.Sprintf("PING %s ack=%t data=%x", frameHeader(f), f.Flags&FlagPingAck != 0, f.Data)
}

func parsePingFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != pingPayloadLength {
		return nil, fmt.Errorf("PING payload must be %d bytes", pingPayloadLength)
	}
	var data [pingPayloadLength]byte
	copy(data[:], payload)
	return PingFrame{Flags: header.Flags, Data: data}, nil
}
