package hpack

import (
	"strings"
	"testing"
)

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

func TestDecodeDetailedWarnsOnLateDynamicTableSizeUpdate(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	fieldBlock, err := codec.Encode([]HeaderField{{Name: ":method", Value: "GET"}})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	block := append(append([]byte(nil), fieldBlock...), appendTableSize(nil, 2048)...)

	report, err := codec.DecodeDetailed(block)
	if err != nil {
		t.Fatalf("DecodeDetailed() error = %v", err)
	}
	if len(report.Fields) != 1 || report.Fields[0].Name != ":method" || report.Fields[0].Value != "GET" {
		t.Fatalf("Fields = %#v", report.Fields)
	}
	if !containsWarning(report.Warnings, "after first header field") {
		t.Fatalf("Warnings = %#v", report.Warnings)
	}
}

func TestDecodeDetailedWarnsAndExpandsOnOversizeDynamicTableUpdate(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	block := appendTableSize(nil, 8192)
	block = append(block, appendNewName(nil, HeaderField{Name: "x-test", Value: "one"}, true)...)

	report, err := codec.DecodeDetailed(block)
	if err != nil {
		t.Fatalf("DecodeDetailed() error = %v", err)
	}
	if !containsWarning(report.Warnings, "exceeds allowed max 4096") {
		t.Fatalf("Warnings = %#v", report.Warnings)
	}
	if len(report.Fields) != 1 || report.Fields[0].Name != "x-test" || report.Fields[0].Value != "one" {
		t.Fatalf("Fields = %#v", report.Fields)
	}

	report, err = codec.DecodeDetailed(appendIndexed(nil, uint64(staticTable.len()+1)))
	if err != nil {
		t.Fatalf("DecodeDetailed(indexed) error = %v", err)
	}
	if len(report.Fields) != 1 || report.Fields[0].Name != "x-test" || report.Fields[0].Value != "one" {
		t.Fatalf("Indexed Fields = %#v", report.Fields)
	}
}

func TestDecodeDetailedInvalidIndexStillErrors(t *testing.T) {
	t.Parallel()

	codec := NewCodec(4096)
	if _, err := codec.DecodeDetailed(appendIndexed(nil, uint64(staticTable.len()+1))); err == nil {
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

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}
