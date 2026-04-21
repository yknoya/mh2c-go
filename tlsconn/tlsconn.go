package tlsconn

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

type VerifyMode int

const (
	VerifyServerCert VerifyMode = iota
	SkipVerifyServerCert
)

type Conn struct {
	raw net.Conn
}

func Dial(ctx context.Context, host string, port uint16, mode VerifyMode, base *tls.Config) (*Conn, error) {
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conf := &tls.Config{
		ServerName:         host,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: mode == SkipVerifyServerCert,
	}
	if base != nil {
		conf = base.Clone()
		if len(conf.NextProtos) == 0 {
			conf.NextProtos = []string{"h2"}
		}
		if conf.ServerName == "" {
			conf.ServerName = host
		}
		if mode == SkipVerifyServerCert {
			conf.InsecureSkipVerify = true
		}
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, conf)
	if err != nil {
		return nil, err
	}
	state := conn.ConnectionState()
	if state.NegotiatedProtocol != "h2" {
		_ = conn.Close()
		return nil, fmt.Errorf("negotiated protocol %q, want h2", state.NegotiatedProtocol)
	}
	return &Conn{raw: conn}, nil
}

func (c *Conn) ReadFull(p []byte) error {
	_, err := io.ReadFull(c.raw, p)
	return err
}

func (c *Conn) Read(p []byte) (int, error) {
	return c.raw.Read(p)
}

func (c *Conn) Write(p []byte) (int, error) {
	return c.raw.Write(p)
}

func (c *Conn) WriteAll(p []byte) error {
	for len(p) > 0 {
		n, err := c.raw.Write(p)
		if err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

func (c *Conn) Close() error {
	return c.raw.Close()
}
