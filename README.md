# mh2c-go

`mh2c-go` is a manual HTTP/2 client library for debugging servers.

This project is not a high-level HTTP client. It is designed for building,
sending, receiving, and inspecting HTTP/2 frames directly, including unusual
or intentionally malformed traffic when needed.

## Status

The current implementation provides:

- manual frame assembly in the `frame` package
- TLS + ALPN `h2` connection handling in `tlsconn`
- a low-level client in `client`
- HPACK support in `hpack`
- a manual debugging CLI in `cmd/mh2c`

## Install

```sh
go build ./...
```

## Test

```sh
go test ./...
```

## CLI

`cmd/mh2c` is a manual HTTP/2 debugging client. It prints received frames and
their payload details instead of hiding protocol activity behind a
curl-style response summary.

### Request Example

```sh
go run ./cmd/mh2c \
  --url https://nghttp2.org/httpbin/headers \
  --header 'user-agent:mh2c-go'
```

### POST Example

```sh
go run ./cmd/mh2c \
  --url https://nghttp2.org/httpbin/post \
  --method POST \
  --header 'content-type: application/json' \
  --data '{"message":"hello from mh2c-go"}'
```

### Ping Example

```sh
go run ./cmd/mh2c \
  --host nghttp2.org \
  --mode ping \
  --ping-data mh2cping
```

### Script Example

```toml
[connection]
url = "https://nghttp2.org/httpbin/headers"
send_goaway = false

[[action]]
type = "preface"

[[action]]
type = "settings"
settings = [
  "ENABLE_PUSH=0",
  "INITIAL_WINDOW_SIZE=65535",
  "HEADER_TABLE_SIZE=8192",
]

[[action]]
type = "receive"
until = "settings"
ack_settings = true

[[action]]
type = "headers"
stream_id = 1
flags = ["END_HEADERS", "END_STREAM"]
headers = [
  ":method: GET",
  ":path: /httpbin/headers",
  ":scheme: https",
  ":authority: nghttp2.org",
  "user-agent: mh2c-go-script",
]

[[action]]
type = "receive"
stream_id = 1
until = "end_stream"
ack_ping = true
```

```sh
go run ./cmd/mh2c --mode script --script-file ./request.toml
```

### Notes

- `--url` overrides `--scheme`, `--host`, `--port`, and `--path`
- `--body-file path/to/file` reads the request body from a file
- `--body-file -` reads the request body from stdin
- `--authority` overrides the `:authority` pseudo-header
- `--mode script --script-file file.toml` executes a scripted frame sequence
- script mode does not auto-send connection preface or SETTINGS; include them explicitly when needed
- supported script actions are `preface`, `settings`, `headers`, `continuation`,
  `data`, `ping`, `goaway`, `window_update`, `rst_stream`, `priority`,
  `push_promise`, `raw`, and `receive`
- the script parser accepts the TOML subset used in the example above:
  strings, integers, booleans, string arrays, `[connection]`, and `[[action]]`
- received frames are printed with payload details such as decoded headers,
  DATA text/hex, SETTINGS entries, and PING payloads

## Package Layout

```text
cmd/mh2c     manual HTTP/2 debugging CLI
client/      connection preface, raw I/O, frame send/receive, HPACK helpers
frame/       manual HTTP/2 frame types and binary encoding/decoding
hpack/       HPACK implementation and wrapper helpers
tlsconn/     TLS connection bootstrap with ALPN h2
internal/    internal wire helpers
```
