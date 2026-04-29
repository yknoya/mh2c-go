package wire

import "testing"

func TestUint16RoundTrip(t *testing.T) {
	t.Parallel()

	raw := AppendUint16(nil, 0x1234)
	got, err := ReadUint16(raw)
	if err != nil {
		t.Fatalf("ReadUint16() error = %v", err)
	}
	if got != 0x1234 {
		t.Fatalf("ReadUint16() = 0x%04x, want 0x1234", got)
	}
}

func TestReadUint16RejectsShortInput(t *testing.T) {
	t.Parallel()

	if _, err := ReadUint16([]byte{0x12}); err == nil {
		t.Fatal("ReadUint16() error = nil, want short input error")
	}
}
