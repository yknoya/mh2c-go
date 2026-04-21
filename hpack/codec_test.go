package hpack

import "testing"

func TestCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	src := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/"},
		{Name: "user-agent", Value: "mh2c-go"},
	}
	block, err := codec.Encode(src)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := codec.Decode(block)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(got) != len(src) {
		t.Fatalf("len(Decode()) = %d, want %d", len(got), len(src))
	}
	for i := range src {
		if got[i].Name != src[i].Name || got[i].Value != src[i].Value {
			t.Fatalf("Decode()[%d] = %#v, want %#v", i, got[i], src[i])
		}
	}
}
