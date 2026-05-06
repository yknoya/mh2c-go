package client

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestSendConnectionPreface(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	c := NewWithConn(left)
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(ConnectionPreface))
		_, _ = io.ReadFull(right, buf)
		done <- buf
	}()

	if err := c.SendConnectionPreface(); err != nil {
		t.Fatalf("SendConnectionPreface() error = %v", err)
	}
	if got := <-done; string(got) != ConnectionPreface {
		t.Fatalf("preface = %q, want %q", string(got), ConnectionPreface)
	}
}

func TestSendAndReceiveFrame(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	c := NewWithConn(left)
	want := frame.SettingsFrame{
		Settings: []frame.Setting{{ID: frame.SettingEnablePush, Value: 0}},
	}
	go func() {
		raw, _ := want.MarshalBinary()
		_, _ = right.Write(raw)
	}()

	got, err := c.ReceiveFrame()
	if err != nil {
		t.Fatalf("ReceiveFrame() error = %v", err)
	}
	typed, ok := got.(frame.SettingsFrame)
	if !ok {
		t.Fatalf("ReceiveFrame() type = %T, want SettingsFrame", got)
	}
	if len(typed.Settings) != 1 || typed.Settings[0].ID != frame.SettingEnablePush {
		t.Fatalf("Settings = %#v", typed.Settings)
	}
}

func TestSendRawFrame(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	c := NewWithConn(left)
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	header := frame.Header{
		Type:     frame.Type(0xf0),
		Flags:    0xaa,
		StreamID: 5,
		Length:   uint32(len(payload)),
	}
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 9+len(payload))
		_, _ = io.ReadFull(right, buf)
		done <- buf
	}()

	if err := c.SendRawFrame(header, payload); err != nil {
		t.Fatalf("SendRawFrame() error = %v", err)
	}
	got := <-done
	if !bytes.Equal(got[9:], payload) {
		t.Fatalf("payload = %x, want %x", got[9:], payload)
	}
}

func TestSendRawFrameExactPreservesHeaderLength(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	c := NewWithConn(left)
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	header := frame.Header{
		Type:     frame.Type(0xf0),
		Flags:    0xaa,
		StreamID: 5,
		Length:   1,
	}
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 9+len(payload))
		_, _ = io.ReadFull(right, buf)
		done <- buf
	}()

	if err := c.SendRawFrameExact(header, payload); err != nil {
		t.Fatalf("SendRawFrameExact() error = %v", err)
	}
	got := <-done
	gotHeader, err := frame.ParseHeader(got[:9])
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}
	if gotHeader.Length != 1 {
		t.Fatalf("Header.Length = %d, want 1", gotHeader.Length)
	}
	if !bytes.Equal(got[9:], payload) {
		t.Fatalf("payload = %x, want %x", got[9:], payload)
	}
}

func TestRequestCodecStartsAtPeerDefaultHeaderTableSize(t *testing.T) {
	t.Parallel()

	c := NewWithConn(testNopConn{}, WithMaxDynamicTableSize(8192))
	fields := []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/"},
		{Name: "x-test", Value: "one"},
	}

	got, err := c.EncodeHeaders(fields)
	if err != nil {
		t.Fatalf("EncodeHeaders() error = %v", err)
	}

	wantCodec := hpack.NewCodec(4096)
	want, err := wantCodec.Encode(fields)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("initial request header block = %x, want %x", got, want)
	}
}

func TestReceiveSettingsUpdatesRequestCodecHeaderTableSize(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	c := NewWithConn(left, WithMaxDynamicTableSize(8192))
	fields := []hpack.HeaderField{{Name: "x-test", Value: "one"}}

	go func() {
		raw, _ := frame.SettingsFrame{
			Settings: []frame.Setting{{ID: frame.SettingHeaderTableSize, Value: 8192}},
		}.MarshalBinary()
		_, _ = right.Write(raw)
	}()

	if _, err := c.ReceiveFrame(); err != nil {
		t.Fatalf("ReceiveFrame() error = %v", err)
	}

	got, err := c.EncodeHeaders(fields)
	if err != nil {
		t.Fatalf("EncodeHeaders() error = %v", err)
	}

	wantCodec := hpack.NewCodec(4096)
	wantCodec.SetEncoderMaxDynamicTableSize(8192)
	want, err := wantCodec.Encode(fields)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("request header block after peer SETTINGS = %x, want %x", got, want)
	}
}

type testNopConn struct{}

func (testNopConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (testNopConn) Write(p []byte) (int, error) { return len(p), nil }
func (testNopConn) Close() error                { return nil }
