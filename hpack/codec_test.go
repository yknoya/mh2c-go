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

func TestDecodeDetailedRejectsLateDynamicTableSizeUpdate(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	fieldBlock, err := codec.Encode([]HeaderField{{Name: "x-test", Value: "one"}})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	block := append(append([]byte(nil), fieldBlock...), tableSizeUpdate2048...)

	if _, err := codec.DecodeDetailed(block); err == nil {
		t.Fatal("DecodeDetailed() error = nil, want late dynamic table size update error")
	}
}

func TestDecodeDetailedRejectsOversizeDynamicTableUpdate(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	if _, err := codec.DecodeDetailed(tableSizeUpdate8192); err == nil {
		t.Fatal("DecodeDetailed() error = nil, want oversize dynamic table update error")
	}
}

func TestDecodeDetailedInvalidIndexStillErrors(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	if _, err := codec.DecodeDetailed(indexStaticTableBeyondEnd); err == nil {
		t.Fatal("DecodeDetailed() error = nil, want invalid index error")
	}
}

func TestDecodeDetailedTruncatedBlockStillErrors(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	block, err := codec.Encode([]HeaderField{{Name: "x-test", Value: "value"}})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if _, err := codec.DecodeDetailed(block[:len(block)-1]); err == nil {
		t.Fatal("DecodeDetailed() error = nil, want truncated headers error")
	}
}

var (
	tableSizeUpdate2048       = []byte{0x3f, 0xe1, 0x0f}
	tableSizeUpdate8192       = []byte{0x3f, 0xe1, 0x3f}
	indexStaticTableBeyondEnd = []byte{0xbe}
)
