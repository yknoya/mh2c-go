package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func TestHTTP2RoundTripAgainstTLSServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("ProtoMajor = %d, want 2", r.ProtoMajor)
		}
		if got := r.Method; got != http.MethodPost {
			t.Errorf("Method = %q, want %q", got, http.MethodPost)
		}
		if got := r.URL.Path; got != "/echo" {
			t.Errorf("Path = %q, want %q", got, "/echo")
		}
		if got := r.Header.Get("X-Test-Request"); got != "sent" {
			t.Errorf("X-Test-Request = %q, want %q", got, "sent")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("x-test-reply", "ok")
		w.Header().Set("content-type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("echo:" + string(body)))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	host, port, authority, tlsConfig := mustServerEndpoint(t, server)
	c, err := New(context.Background(), host, port, WithTLSConfig(tlsConfig))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	if err := c.SendConnectionPreface(); err != nil {
		t.Fatalf("SendConnectionPreface() error = %v", err)
	}
	if err := c.SendFrame(frame.SettingsFrame{
		Settings: []frame.Setting{
			{ID: frame.SettingEnablePush, Value: 0},
			{ID: frame.SettingInitialWindowSize, Value: 65535},
		},
	}); err != nil {
		t.Fatalf("SendFrame(SETTINGS) error = %v", err)
	}

	if err := acknowledgePeerSettings(t, c); err != nil {
		t.Fatalf("acknowledgePeerSettings() error = %v", err)
	}

	block, err := c.EncodeHeaders([]hpack.HeaderField{
		{Name: ":method", Value: http.MethodPost},
		{Name: ":path", Value: "/echo"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: authority},
		{Name: "content-length", Value: "5"},
		{Name: "x-test-request", Value: "sent"},
	})
	if err != nil {
		t.Fatalf("EncodeHeaders() error = %v", err)
	}
	if err := c.SendFrame(frame.HeadersFrame{
		StreamID:      1,
		Flags:         frame.FlagHeadersEndHeaders,
		BlockFragment: block,
	}); err != nil {
		t.Fatalf("SendFrame(HEADERS) error = %v", err)
	}
	if err := c.SendFrame(frame.DataFrame{
		StreamID: 1,
		Flags:    frame.FlagDataEndStream,
		Data:     []byte("hello"),
	}); err != nil {
		t.Fatalf("SendFrame(DATA) error = %v", err)
	}

	fields, body, err := readResponse(t, c, 1)
	if err != nil {
		t.Fatalf("readResponse() error = %v", err)
	}
	if got := fieldValue(fields, ":status"); got != "201" {
		t.Fatalf(":status = %q, want %q", got, "201")
	}
	if got := fieldValue(fields, "x-test-reply"); got != "ok" {
		t.Fatalf("x-test-reply = %q, want %q", got, "ok")
	}
	if got := string(body); got != "echo:hello" {
		t.Fatalf("body = %q, want %q", got, "echo:hello")
	}
}

func mustServerEndpoint(t *testing.T, server *httptest.Server) (string, uint16, string, *tls.Config) {
	t.Helper()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	host, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	portValue, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		t.Fatalf("ParseUint() error = %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())

	return host, uint16(portValue), u.Host, &tls.Config{
		RootCAs:    pool,
		ServerName: host,
	}
}

func acknowledgePeerSettings(t *testing.T, c *Client) error {
	t.Helper()

	for {
		f, err := c.ReceiveFrame()
		if err != nil {
			return err
		}
		settings, ok := f.(frame.SettingsFrame)
		if !ok {
			continue
		}
		if settings.Flags&frame.FlagSettingsAck != 0 {
			continue
		}
		return c.SendFrame(frame.SettingsFrame{Flags: frame.FlagSettingsAck})
	}
}

func readResponse(t *testing.T, c *Client, streamID uint32) ([]hpack.HeaderField, []byte, error) {
	t.Helper()

	var (
		fields []hpack.HeaderField
		body   []byte
	)
	for {
		f, err := c.ReceiveFrame()
		if err != nil {
			return nil, nil, err
		}
		switch typed := f.(type) {
		case frame.SettingsFrame:
			if typed.Flags&frame.FlagSettingsAck == 0 {
				if err := c.SendFrame(frame.SettingsFrame{Flags: frame.FlagSettingsAck}); err != nil {
					return nil, nil, err
				}
			}
		case frame.HeadersFrame:
			if typed.StreamID != streamID {
				continue
			}
			decoded, err := c.DecodeHeaders(typed.BlockFragment)
			if err != nil {
				return nil, nil, err
			}
			fields = append(fields, decoded...)
			if typed.Flags&frame.FlagHeadersEndStream != 0 {
				return fields, body, nil
			}
		case frame.ContinuationFrame:
			return nil, nil, fmt.Errorf("unexpected CONTINUATION frame on stream %d", typed.StreamID)
		case frame.DataFrame:
			if typed.StreamID != streamID {
				continue
			}
			body = append(body, typed.Data...)
			if typed.Flags&frame.FlagDataEndStream != 0 {
				return fields, body, nil
			}
		}
	}
}

func fieldValue(fields []hpack.HeaderField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}
