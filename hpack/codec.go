package hpack

import (
	"bytes"

	xhpack "golang.org/x/net/http2/hpack"
)

const initialHeaderTableSize = 4096

type HeaderField = xhpack.HeaderField
type Decoder = xhpack.Decoder
type Encoder = xhpack.Encoder
type DecodingError = xhpack.DecodingError
type InvalidIndexError = xhpack.InvalidIndexError

var (
	ErrStringLength = xhpack.ErrStringLength
	NewDecoder      = xhpack.NewDecoder
	NewEncoder      = xhpack.NewEncoder
)

type DecodeReport struct {
	Fields   []HeaderField
	Warnings []string
}

type Codec struct {
	maxSize uint32
	buf     *bytes.Buffer
	encoder *Encoder
	decoder *Decoder
}

func NewCodec(maxDynamicTableSize uint32) *Codec {
	return newCodec(maxDynamicTableSize, maxDynamicTableSize, maxDynamicTableSize)
}

func NewRequestCodec() *Codec {
	return newCodec(initialHeaderTableSize, initialHeaderTableSize, initialHeaderTableSize)
}

func NewResponseCodec(allowedMaxDynamicTableSize uint32) *Codec {
	return newCodec(initialHeaderTableSize, initialHeaderTableSize, allowedMaxDynamicTableSize)
}

func newCodec(encoderMaxDynamicTableSize, decoderDynamicTableSize, decoderAllowedMaxDynamicTableSize uint32) *Codec {
	buf := bytes.NewBuffer(nil)
	enc := NewEncoder(buf)
	enc.SetMaxDynamicTableSizeLimit(encoderMaxDynamicTableSize)
	enc.SetMaxDynamicTableSize(encoderMaxDynamicTableSize)
	dec := NewDecoder(decoderDynamicTableSize, nil)
	dec.SetAllowedMaxDynamicTableSize(decoderAllowedMaxDynamicTableSize)
	dec.SetMaxDynamicTableSize(decoderDynamicTableSize)
	return &Codec{
		maxSize: encoderMaxDynamicTableSize,
		buf:     buf,
		encoder: enc,
		decoder: dec,
	}
}

func (c *Codec) SetMaxDynamicTableSize(v uint32) {
	c.maxSize = v
	c.encoder.SetMaxDynamicTableSizeLimit(v)
	c.encoder.SetMaxDynamicTableSize(v)
	c.decoder.SetAllowedMaxDynamicTableSize(v)
	c.decoder.SetMaxDynamicTableSize(v)
}

func (c *Codec) SetEncoderMaxDynamicTableSize(v uint32) {
	c.maxSize = v
	c.encoder.SetMaxDynamicTableSizeLimit(v)
	c.encoder.SetMaxDynamicTableSize(v)
}

func (c *Codec) SetDecoderAllowedMaxDynamicTableSize(v uint32) {
	c.decoder.SetAllowedMaxDynamicTableSize(v)
}

func (c *Codec) SetDecoderMaxDynamicTableSize(v uint32) {
	c.decoder.SetMaxDynamicTableSize(v)
}

func (c *Codec) MaxDynamicTableSize() uint32 {
	return c.maxSize
}

func (c *Codec) Encode(fields []HeaderField) ([]byte, error) {
	c.buf.Reset()
	for _, field := range fields {
		if err := c.encoder.WriteField(field); err != nil {
			return nil, err
		}
	}
	return append([]byte(nil), c.buf.Bytes()...), nil
}

func (c *Codec) Decode(block []byte) ([]HeaderField, error) {
	report, err := c.DecodeDetailed(block)
	if err != nil {
		return nil, err
	}
	return report.Fields, nil
}

func (c *Codec) DecodeDetailed(block []byte) (DecodeReport, error) {
	fields, err := c.decoder.DecodeFull(block)
	if err != nil {
		return DecodeReport{}, err
	}
	return DecodeReport{Fields: fields}, nil
}
