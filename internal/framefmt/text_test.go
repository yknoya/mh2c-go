package framefmt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestWriteTextFrameIncludesFrameDetails(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := WriteTextFrame(&out, TextFrame{
		Prefix:     "<<",
		Frame:      frame.DataFrame{StreamID: 1, Flags: frame.FlagDataEndStream, Data: []byte("hello")},
		DataFormat: DataFormatBoth,
	})
	if err != nil {
		t.Fatalf("WriteTextFrame() error = %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"<< DATA stream=1 len=5 type=DATA(0x00) flags=0x01 end_stream=true data=5",
		"data-length: 5",
		"data-hex: 68656c6c6f",
		"data-text: \"hello\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}

func TestWriteTextFrameIncludesDecodedHeadersAndWarnings(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := WriteTextFrame(&out, TextFrame{
		Prefix:          ">>",
		Frame:           frame.HeadersFrame{StreamID: 3, Flags: frame.FlagHeadersEndHeaders, BlockFragment: []byte{0x82}},
		Headers:         []hpack.HeaderField{{Name: ":method", Value: "GET"}},
		Warnings:        []string{"demo warning"},
		ShowHeaderBlock: true,
		DataFormat:      DataFormatHex,
	})
	if err != nil {
		t.Fatalf("WriteTextFrame() error = %v", err)
	}

	text := out.String()
	for _, want := range []string{
		">> HEADERS stream=3",
		"header-block-fragment: 82",
		"header-warning: demo warning",
		"header :method: GET",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}
