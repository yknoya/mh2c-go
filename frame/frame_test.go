package frame

import (
	"bytes"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	t.Parallel()

	header := Header{Length: 12, Type: TypeHeaders, Flags: FlagHeadersEndHeaders, StreamID: 3}
	raw, err := header.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	got, err := ParseHeader(raw)
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	if got != header {
		t.Fatalf("ParseHeader() = %#v, want %#v", got, header)
	}
}

func TestSettingsFrameRoundTrip(t *testing.T) {
	t.Parallel()

	src := SettingsFrame{
		Settings: []Setting{
			{ID: SettingEnablePush, Value: 0},
			{ID: SettingInitialWindowSize, Value: 65535},
		},
	}
	raw, err := src.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := ParseHeader(raw[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[9:])
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	typed, ok := got.(SettingsFrame)
	if !ok {
		t.Fatalf("Unmarshal() type = %T, want SettingsFrame", got)
	}
	if len(typed.Settings) != 2 {
		t.Fatalf("len(Settings) = %d, want 2", len(typed.Settings))
	}
}

func TestHeadersFrameRoundTripWithPriority(t *testing.T) {
	t.Parallel()

	src := HeadersFrame{
		StreamID:      1,
		Flags:         FlagHeadersEndHeaders | FlagHeadersPriority,
		Priority:      &PriorityParam{Exclusive: true, StreamDep: 3, Weight: 10},
		BlockFragment: []byte{0x82, 0x86},
	}
	raw, err := src.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := ParseHeader(raw[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[9:])
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	typed, ok := got.(HeadersFrame)
	if !ok {
		t.Fatalf("Unmarshal() type = %T, want HeadersFrame", got)
	}
	if !bytes.Equal(typed.BlockFragment, src.BlockFragment) {
		t.Fatalf("BlockFragment = %x, want %x", typed.BlockFragment, src.BlockFragment)
	}
	if typed.Priority == nil || typed.Priority.StreamDep != 3 || !typed.Priority.Exclusive {
		t.Fatalf("Priority = %#v", typed.Priority)
	}
}

func TestPriorityFrameRoundTrip(t *testing.T) {
	t.Parallel()

	src := PriorityFrame{
		StreamID:  7,
		Exclusive: true,
		StreamDep: 3,
		Weight:    15,
	}
	raw, err := src.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := ParseHeader(raw[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[9:])
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	typed, ok := got.(PriorityFrame)
	if !ok {
		t.Fatalf("Unmarshal() type = %T, want PriorityFrame", got)
	}
	if typed != src {
		t.Fatalf("PriorityFrame = %#v, want %#v", typed, src)
	}
}

func TestPushPromiseFrameRoundTrip(t *testing.T) {
	t.Parallel()

	src := PushPromiseFrame{
		StreamID:         1,
		Flags:            FlagPushPromiseEndHeaders | FlagPushPromisePadded,
		PadLength:        2,
		PromisedStreamID: 2,
		BlockFragment:    []byte{0x82, 0x86},
	}
	raw, err := src.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := ParseHeader(raw[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[9:])
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	typed, ok := got.(PushPromiseFrame)
	if !ok {
		t.Fatalf("Unmarshal() type = %T, want PushPromiseFrame", got)
	}
	if typed.StreamID != src.StreamID || typed.Flags != src.Flags || typed.PadLength != src.PadLength || typed.PromisedStreamID != src.PromisedStreamID {
		t.Fatalf("PushPromiseFrame = %#v, want %#v", typed, src)
	}
	if !bytes.Equal(typed.BlockFragment, src.BlockFragment) {
		t.Fatalf("BlockFragment = %x, want %x", typed.BlockFragment, src.BlockFragment)
	}
}

func TestUnknownFrameBecomesRawFrame(t *testing.T) {
	t.Parallel()

	header := Header{Length: 3, Type: Type(0xfe), Flags: 0xaa, StreamID: 9}
	got, err := Unmarshal(header, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	raw, ok := got.(RawFrame)
	if !ok {
		t.Fatalf("Unmarshal() type = %T, want RawFrame", got)
	}
	if raw.Header().Type != Type(0xfe) {
		t.Fatalf("Type = 0x%02x, want 0xfe", raw.Header().Type)
	}
}
