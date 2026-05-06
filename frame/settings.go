package frame

import (
	"fmt"
	"strings"

	"github.com/yknoya/mh2c-go/internal/wire"
)

const FlagSettingsAck uint8 = 0x1

const (
	settingIDLength    = 2
	settingValueLength = 4
	settingEntryLength = settingIDLength + settingValueLength
)

type SettingID uint16

const (
	SettingHeaderTableSize      SettingID = 0x1
	SettingEnablePush           SettingID = 0x2
	SettingMaxConcurrentStreams SettingID = 0x3
	SettingInitialWindowSize    SettingID = 0x4
	SettingMaxFrameSize         SettingID = 0x5
	SettingMaxHeaderListSize    SettingID = 0x6
)

func (id SettingID) String() string {
	switch id {
	case SettingHeaderTableSize:
		return "HEADER_TABLE_SIZE"
	case SettingEnablePush:
		return "ENABLE_PUSH"
	case SettingMaxConcurrentStreams:
		return "MAX_CONCURRENT_STREAMS"
	case SettingInitialWindowSize:
		return "INITIAL_WINDOW_SIZE"
	case SettingMaxFrameSize:
		return "MAX_FRAME_SIZE"
	case SettingMaxHeaderListSize:
		return "MAX_HEADER_LIST_SIZE"
	default:
		return fmt.Sprintf("0x%04x", uint16(id))
	}
}

type Setting struct {
	ID    SettingID
	Value uint32
}

type SettingsFrame struct {
	FrameHeader Header
	Settings    []Setting
}

func NewSettingsFrame(flags uint8, settings []Setting) SettingsFrame {
	frame := SettingsFrame{
		FrameHeader: Header{Type: TypeSettings, Flags: flags, StreamID: 0},
		Settings:    append([]Setting(nil), settings...),
	}
	frame.FrameHeader.Length = uint32(len(frame.Payload()))
	return frame
}

func (f SettingsFrame) Header() Header {
	return f.FrameHeader
}

func (f SettingsFrame) Payload() []byte {
	payload := make([]byte, 0, len(f.Settings)*settingEntryLength)
	for _, s := range f.Settings {
		payload = wire.AppendUint16(payload, uint16(s.ID))
		payload = wire.AppendUint32(payload, s.Value)
	}
	return payload
}

func (f SettingsFrame) MarshalBinary() ([]byte, error) {
	return encode(f.Header(), f.Payload())
}

func (f SettingsFrame) String() string {
	settings := make([]string, 0, len(f.Settings))
	for _, setting := range f.Settings {
		settings = append(settings, fmt.Sprintf("%s=%d", setting.ID, setting.Value))
	}
	return fmt.Sprintf("SETTINGS %s ack=%t settings=[%s]", frameHeader(f), f.Header().Flags&FlagSettingsAck != 0, strings.Join(settings, " "))
}

func parseSettingsFrame(header Header, payload []byte) (Frame, error) {
	if len(payload)%settingEntryLength != 0 {
		return nil, fmt.Errorf("SETTINGS payload must be multiple of %d bytes", settingEntryLength)
	}
	frame := SettingsFrame{FrameHeader: header}
	for i := 0; i < len(payload); i += settingEntryLength {
		id, err := wire.ReadUint16(payload[i : i+settingIDLength])
		if err != nil {
			return nil, err
		}
		valueStart := i + settingIDLength
		value, err := wire.ReadUint32(payload[valueStart : valueStart+settingValueLength])
		if err != nil {
			return nil, err
		}
		frame.Settings = append(frame.Settings, Setting{
			ID:    SettingID(id),
			Value: value,
		})
	}
	return frame, nil
}
