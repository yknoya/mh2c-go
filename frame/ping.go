package frame

import "fmt"

const FlagPingAck uint8 = 0x1

type PingFrame struct {
	Flags uint8
	Data  [8]byte
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
	return fmt.Sprintf("PING flags=0x%02x", f.Flags)
}

func parsePingFrame(header Header, payload []byte) (Frame, error) {
	if len(payload) != 8 {
		return nil, fmt.Errorf("PING payload must be 8 bytes")
	}
	var data [8]byte
	copy(data[:], payload)
	return PingFrame{Flags: header.Flags, Data: data}, nil
}
