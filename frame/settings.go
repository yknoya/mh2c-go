package frame

import "fmt"

const FlagSettingsAck uint8 = 0x1

type SettingID uint16

const (
	SettingHeaderTableSize      SettingID = 0x1
	SettingEnablePush           SettingID = 0x2
	SettingMaxConcurrentStreams SettingID = 0x3
	SettingInitialWindowSize    SettingID = 0x4
	SettingMaxFrameSize         SettingID = 0x5
	SettingMaxHeaderListSize    SettingID = 0x6
)

type Setting struct {
	ID    SettingID
	Value uint32
}

type SettingsFrame struct {
	Flags    uint8
	Settings []Setting
}

func (f SettingsFrame) Header() Header {
	return Header{Type: TypeSettings, Flags: f.Flags, StreamID: 0}
}

func (f SettingsFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.Settings)*6)
	for _, s := range f.Settings {
		payload = append(payload, byte(s.ID>>8), byte(s.ID))
		payload = append(payload, byte(s.Value>>24), byte(s.Value>>16), byte(s.Value>>8), byte(s.Value))
	}
	return payload
}

func (f SettingsFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f SettingsFrame) String() string {
	return fmt.Sprintf("SETTINGS flags=0x%02x entries=%d", f.Flags, len(f.Settings))
}

func parseSettingsFrame(header Header, payload []byte) (Frame, error) {
	if len(payload)%6 != 0 {
		return nil, fmt.Errorf("SETTINGS payload must be multiple of 6 bytes")
	}
	frame := SettingsFrame{Flags: header.Flags}
	for i := 0; i < len(payload); i += 6 {
		frame.Settings = append(frame.Settings, Setting{
			ID:    SettingID(uint16(payload[i])<<8 | uint16(payload[i+1])),
			Value: uint32(payload[i+2])<<24 | uint32(payload[i+3])<<16 | uint32(payload[i+4])<<8 | uint32(payload[i+5]),
		})
	}
	return frame, nil
}
