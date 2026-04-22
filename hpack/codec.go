package hpack

import (
	"bytes"
)

type Codec struct {
	maxSize uint32
	buf     *bytes.Buffer
	encoder *Encoder
	decoder *Decoder
}

func NewCodec(maxDynamicTableSize uint32) *Codec {
	buf := bytes.NewBuffer(nil)
	enc := NewEncoder(buf)
	enc.SetMaxDynamicTableSizeLimit(maxDynamicTableSize)
	enc.SetMaxDynamicTableSize(maxDynamicTableSize)
	dec := NewDecoder(maxDynamicTableSize, nil)
	dec.SetAllowedMaxDynamicTableSize(maxDynamicTableSize)
	dec.SetMaxDynamicTableSize(maxDynamicTableSize)
	return &Codec{
		maxSize: maxDynamicTableSize,
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
	return c.decoder.DecodeFullDetailed(block)
}
