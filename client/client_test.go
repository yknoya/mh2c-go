package client

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/yknoya/mh2c-go/frame"
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
