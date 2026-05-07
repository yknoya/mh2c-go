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

	src := NewSettingsFrame(0, []Setting{
		{ID: SettingEnablePush, Value: 0},
		{ID: SettingInitialWindowSize, Value: 65535},
	})
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

	src := NewHeadersFrame(1, FlagHeadersEndHeaders|FlagHeadersPriority, []byte{0x82, 0x86})
	src.Priority = &PriorityParam{Exclusive: true, StreamDep: 3, Weight: 10}
	src.FrameHeader.Length = uint32(len(src.Payload()))
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

	src := NewPriorityFrame(7, true, 3, 15)
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

	src := NewPushPromiseFrame(1, FlagPushPromiseEndHeaders|FlagPushPromisePadded, 2, []byte{0x82, 0x86})
	src.PadLength = 2
	src.FrameHeader.Length = uint32(len(src.Payload()))
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
	if typed.Header().StreamID != src.Header().StreamID || typed.Header().Flags != src.Header().Flags || typed.PadLength != src.PadLength || typed.PromisedStreamID != src.PromisedStreamID {
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

func TestTypedFrameConstructorsSetCompleteHeaders(t *testing.T) {
	t.Parallel()

	pingData := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	tests := []struct {
		name string
		got  Frame
		want Header
	}{
		{
			name: "settings",
			got:  NewSettingsFrame(FlagSettingsAck, nil),
			want: Header{Type: TypeSettings, Flags: FlagSettingsAck, Length: 0},
		},
		{
			name: "headers",
			got:  NewHeadersFrame(1, FlagHeadersEndHeaders, []byte{0x82, 0x86}),
			want: Header{Type: TypeHeaders, Flags: FlagHeadersEndHeaders, StreamID: 1, Length: 2},
		},
		{
			name: "continuation",
			got:  NewContinuationFrame(1, FlagContinuationEndHeaders, []byte{0x82}),
			want: Header{Type: TypeContinuation, Flags: FlagContinuationEndHeaders, StreamID: 1, Length: 1},
		},
		{
			name: "push_promise",
			got:  NewPushPromiseFrame(1, FlagPushPromiseEndHeaders, 2, []byte{0x82}),
			want: Header{Type: TypePushPromise, Flags: FlagPushPromiseEndHeaders, StreamID: 1, Length: 5},
		},
		{
			name: "ping",
			got:  NewPingFrame(FlagPingAck, pingData),
			want: Header{Type: TypePing, Flags: FlagPingAck, Length: 8},
		},
		{
			name: "goaway",
			got:  NewGoAwayFrame(3, ErrNo, []byte{0xaa, 0xbb}),
			want: Header{Type: TypeGoAway, Length: 10},
		},
		{
			name: "priority",
			got:  NewPriorityFrame(7, true, 3, 15),
			want: Header{Type: TypePriority, StreamID: 7, Length: 5},
		},
		{
			name: "rst_stream",
			got:  NewRSTStreamFrame(9, ErrNo),
			want: Header{Type: TypeRSTStream, StreamID: 9, Length: 4},
		},
		{
			name: "window_update",
			got:  NewWindowUpdateFrame(11, 1024),
			want: Header{Type: TypeWindowUpdate, StreamID: 11, Length: 4},
		},
	}

	for _, tt := range tests {
		if got := tt.got.Header(); got != tt.want {
			t.Fatalf("%s Header() = %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func TestTypedFrameConstructorsCopySliceInputs(t *testing.T) {
	t.Parallel()

	settings := []Setting{{ID: SettingEnablePush, Value: 0}}
	settingsFrame := NewSettingsFrame(0, settings)
	settings[0].Value = 1
	if settingsFrame.Settings[0].Value != 0 {
		t.Fatalf("SettingsFrame settings = %#v, want copied input", settingsFrame.Settings)
	}

	block := []byte{0x82, 0x86}
	headersFrame := NewHeadersFrame(1, FlagHeadersEndHeaders, block)
	continuationFrame := NewContinuationFrame(1, FlagContinuationEndHeaders, block)
	pushPromiseFrame := NewPushPromiseFrame(1, FlagPushPromiseEndHeaders, 2, block)
	block[0] = 0xff
	if headersFrame.BlockFragment[0] != 0x82 || continuationFrame.BlockFragment[0] != 0x82 || pushPromiseFrame.BlockFragment[0] != 0x82 {
		t.Fatalf("header block constructors did not copy input")
	}

	debug := []byte{0xaa, 0xbb}
	goAwayFrame := NewGoAwayFrame(3, ErrNo, debug)
	debug[0] = 0xff
	if goAwayFrame.DebugData[0] != 0xaa {
		t.Fatalf("GoAwayFrame debug data = %x, want copied input", goAwayFrame.DebugData)
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

	settings := NewSettingsFrame(0, []Setting{
		{ID: SettingHeaderTableSize, Value: 4096},
		{ID: SettingEnablePush, Value: 0},
	})
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
		"data_bytes=5",
	} {
		if got := data.String(); !strings.Contains(got, want) {
			t.Fatalf("DataFrame.String() = %q, want %q", got, want)
		}
	}

	headers := NewHeadersFrame(1, FlagHeadersEndHeaders, []byte{0x82, 0x86})
	if got := headers.String(); !strings.Contains(got, "header_block_fragment_bytes=2") {
		t.Fatalf("HeadersFrame.String() = %q, want header block fragment byte count", got)
	}

	continuation := NewContinuationFrame(1, FlagContinuationEndHeaders, []byte{0x82})
	if got := continuation.String(); !strings.Contains(got, "header_block_fragment_bytes=1") {
		t.Fatalf("ContinuationFrame.String() = %q, want header block fragment byte count", got)
	}

	pushPromise := NewPushPromiseFrame(1, FlagPushPromiseEndHeaders, 2, []byte{0x82})
	if got := pushPromise.String(); !strings.Contains(got, "header_block_fragment_bytes=1") {
		t.Fatalf("PushPromiseFrame.String() = %q, want header block fragment byte count", got)
	}
}
