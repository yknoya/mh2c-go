package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	if _, err := c.SendFrame(frame.NewSettingsFrame(0, []frame.Setting{
		{ID: frame.SettingEnablePush, Value: 0},
		{ID: frame.SettingInitialWindowSize, Value: 65535},
	})); err != nil {
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
	if _, err := c.SendFrame(frame.NewHeadersFrame(1, frame.FlagHeadersEndHeaders, block)); err != nil {
		t.Fatalf("SendFrame(HEADERS) error = %v", err)
	}
	if _, err := c.SendFrame(frame.NewDataFrame(1, frame.FlagDataEndStream, []byte("hello"))); err != nil {
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
		event, err := c.ReceiveFrame()
		if err != nil {
			return err
		}
		settings, ok := event.Frame.(frame.SettingsFrame)
		if !ok {
			continue
		}
		if settings.Header().Flags&frame.FlagSettingsAck != 0 {
			continue
		}
		_, err = c.SendFrame(frame.NewSettingsFrame(frame.FlagSettingsAck, nil))
		return err
	}
}

func readResponse(t *testing.T, c *Client, streamID uint32) ([]hpack.HeaderField, []byte, error) {
	t.Helper()

	var (
		fields []hpack.HeaderField
		body   []byte
	)
	for {
		event, err := c.ReceiveFrame()
		if err != nil {
			return nil, nil, err
		}
		switch typed := event.Frame.(type) {
		case frame.SettingsFrame:
			if typed.Header().Flags&frame.FlagSettingsAck == 0 {
				if _, err := c.SendFrame(frame.NewSettingsFrame(frame.FlagSettingsAck, nil)); err != nil {
					return nil, nil, err
				}
			}
		case frame.HeadersFrame:
			if typed.Header().StreamID != streamID {
				continue
			}
			if event.DecodeError != nil {
				return nil, nil, event.DecodeError
			}
			fields = append(fields, event.Headers...)
			if event.HeaderBlockComplete && event.HeaderBlockEndStream {
				return fields, body, nil
			}
		case frame.ContinuationFrame:
			if event.DecodeError != nil {
				return nil, nil, event.DecodeError
			}
			if event.HeaderBlockStreamID == streamID {
				fields = append(fields, event.Headers...)
				if event.HeaderBlockComplete && event.HeaderBlockEndStream {
					return fields, body, nil
				}
			}
		case frame.DataFrame:
			if typed.Header().StreamID != streamID {
				continue
			}
			body = append(body, typed.Data...)
			if typed.Header().Flags&frame.FlagDataEndStream != 0 {
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
