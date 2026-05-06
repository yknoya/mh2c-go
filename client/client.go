package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
	"github.com/yknoya/mh2c-go/internal/wire"
	"github.com/yknoya/mh2c-go/tlsconn"
)

const ConnectionPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

type Option func(*config)

type config struct {
	tlsConfig  *tls.Config
	verifyMode tlsconn.VerifyMode
	maxTable   uint32
}

func WithTLSConfig(cfg *tls.Config) Option {
	return func(c *config) {
		c.tlsConfig = cfg
	}
}

func WithVerifyMode(mode tlsconn.VerifyMode) Option {
	return func(c *config) {
		c.verifyMode = mode
	}
}

func WithInsecureSkipVerify() Option {
	return func(c *config) {
		c.verifyMode = tlsconn.SkipVerifyServerCert
	}
}

func WithMaxDynamicTableSize(v uint32) Option {
	return func(c *config) {
		c.maxTable = v
	}
}

type Client struct {
	conn          io.ReadWriteCloser
	requestCodec  *hpack.Codec
	responseCodec *hpack.Codec
}

func New(ctx context.Context, host string, port uint16, opts ...Option) (*Client, error) {
	cfg := config{
		verifyMode: tlsconn.VerifyServerCert,
		maxTable:   4096,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	conn, err := tlsconn.Dial(ctx, host, port, cfg.verifyMode, cfg.tlsConfig)
	if err != nil {
		return nil, err
	}
	return NewWithConn(conn, opts...), nil
}

func NewWithConn(conn io.ReadWriteCloser, opts ...Option) *Client {
	cfg := config{
		verifyMode: tlsconn.VerifyServerCert,
		maxTable:   4096,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Client{
		conn:          conn,
		requestCodec:  hpack.NewRequestCodec(),
		responseCodec: hpack.NewResponseCodec(cfg.maxTable),
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) SendRaw(data []byte) error {
	if c.conn == nil {
		return errors.New("client connection is nil")
	}
	for len(data) > 0 {
		n, err := c.conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (c *Client) ReceiveRaw(n int) ([]byte, error) {
	if c.conn == nil {
		return nil, errors.New("client connection is nil")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *Client) SendConnectionPreface() error {
	return c.SendRaw([]byte(ConnectionPreface))
}

func (c *Client) SendFrame(f frame.Frame) error {
	raw, err := f.MarshalBinary()
	if err != nil {
		return err
	}
	return c.SendRaw(raw)
}

func (c *Client) SendRawFrame(header frame.Header, payload []byte) error {
	if uint32(len(payload)) != header.Length {
		header.Length = uint32(len(payload))
	}
	return c.SendRawFrameExact(header, payload)
}

func (c *Client) SendRawFrameExact(header frame.Header, payload []byte) error {
	head, err := header.MarshalBinary()
	if err != nil {
		return err
	}
	return c.SendRaw(append(head, payload...))
}

func (c *Client) ReceiveFrame() (frame.Frame, error) {
	headBytes, err := c.ReceiveRaw(wire.FrameHeaderLength)
	if err != nil {
		return nil, err
	}
	header, err := frame.ParseHeader(headBytes)
	if err != nil {
		return nil, err
	}
	payload, err := c.ReceiveRaw(int(header.Length))
	if err != nil {
		return nil, err
	}
	f, err := frame.Unmarshal(header, payload)
	if err != nil {
		return nil, err
	}
	c.applyFrame(f)
	return f, nil
}

func (c *Client) EncodeHeaders(fields []hpack.HeaderField) ([]byte, error) {
	return c.requestCodec.Encode(fields)
}

func (c *Client) DecodeHeaders(block []byte) ([]hpack.HeaderField, error) {
	return c.responseCodec.Decode(block)
}

func (c *Client) DecodeHeadersDetailed(block []byte) (hpack.DecodeReport, error) {
	return c.responseCodec.DecodeDetailed(block)
}

func (c *Client) RequestCodec() *hpack.Codec {
	return c.requestCodec
}

func (c *Client) ResponseCodec() *hpack.Codec {
	return c.responseCodec
}

func (c *Client) applyFrame(f frame.Frame) {
	switch typed := f.(type) {
	case frame.SettingsFrame:
		for _, setting := range typed.Settings {
			if setting.ID == frame.SettingHeaderTableSize {
				c.requestCodec.SetMaxDynamicTableSize(setting.Value)
			}
		}
	}
}

func MustHeadersFrame(streamID uint32, flags uint8, block []byte) frame.HeadersFrame {
	return frame.NewHeadersFrame(streamID, flags, block)
}

func DebugFrameString(f frame.Frame) string {
	switch typed := f.(type) {
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%T len=%d", f, len(f.Payload()))
	}
}
