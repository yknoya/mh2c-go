package frame

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/internal/wire"
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
	header, err := ParseHeader(raw[:wire.FrameHeaderLength])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[wire.FrameHeaderLength:])
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
	header, err := ParseHeader(raw[:wire.FrameHeaderLength])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[wire.FrameHeaderLength:])
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
	header, err := ParseHeader(raw[:wire.FrameHeaderLength])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[wire.FrameHeaderLength:])
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
	header, err := ParseHeader(raw[:wire.FrameHeaderLength])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	got, err := Unmarshal(header, raw[wire.FrameHeaderLength:])
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

func TestRawFrameExactPartsPreserveHeaderLength(t *testing.T) {
	t.Parallel()

	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	normalized := RawFrameFromParts(Header{Type: Type(0xfe), Flags: 0xaa, StreamID: 9, Length: 1}, payload)
	if got := normalized.Header().Length; got != uint32(len(payload)) {
		t.Fatalf("RawFrameFromParts length = %d, want %d", got, len(payload))
	}

	exact := RawFrameFromExactParts(Header{Type: Type(0xfe), Flags: 0xaa, StreamID: 9, Length: 1}, payload)
	raw, err := exact.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	header, err := ParseHeader(raw[:wire.FrameHeaderLength])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	if header.Length != 1 {
		t.Fatalf("Header.Length = %d, want 1", header.Length)
	}
	if !bytes.Equal(raw[wire.FrameHeaderLength:], payload) {
		t.Fatalf("payload = %x, want %x", raw[wire.FrameHeaderLength:], payload)
	}
}

func TestNewDataFrameCopiesDataAndSetsHeader(t *testing.T) {
	t.Parallel()

	data := []byte("hello")
	got := NewDataFrame(3, FlagDataEndStream, data)
	data[0] = 'H'

	if got.FrameHeader.Type != TypeData || got.FrameHeader.Length != 5 {
		t.Fatalf("FrameHeader = %#v", got.FrameHeader)
	}
	header := got.Header()
	if header.Type != TypeData || header.StreamID != 3 || header.Flags != FlagDataEndStream {
		t.Fatalf("Header() = %#v", header)
	}
	if string(got.Data) != "hello" {
		t.Fatalf("Data = %q, want hello", got.Data)
	}
}

func TestFrameStringIncludesHeaderAndSemantics(t *testing.T) {
	t.Parallel()

	settings := SettingsFrame{
		Settings: []Setting{
			{ID: SettingHeaderTableSize, Value: 4096},
			{ID: SettingEnablePush, Value: 0},
		},
	}
	for _, want := range []string{
		"SETTINGS stream=0",
		"len=12",
		"type=SETTINGS(0x04)",
		"settings=[HEADER_TABLE_SIZE=4096 ENABLE_PUSH=0]",
	} {
		if got := settings.String(); !strings.Contains(got, want) {
			t.Fatalf("SettingsFrame.String() = %q, want %q", got, want)
		}
	}

	data := NewDataFrame(1, FlagDataEndStream, []byte("hello"))
	for _, want := range []string{
		"DATA stream=1",
		"len=5",
		"end_stream=true",
		"data=5",
	} {
		if got := data.String(); !strings.Contains(got, want) {
			t.Fatalf("DataFrame.String() = %q, want %q", got, want)
		}
	}
}
