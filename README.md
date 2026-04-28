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
- HPACK support via `golang.org/x/net/http2/hpack`, wrapped by the local `hpack` package
- a manual debugging CLI in `cmd/mh2c`

Reusable CLI examples live under [`examples/`](./examples), including observe
flows, script files, and a small `jsonl` consumer.

## Install

```sh
make build-cli
./bin/mh2c --help
```

Or install it into your Go bin directory:

```sh
go install ./cmd/mh2c
mh2c --help
```

The repository-wide build still works as before:

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
./bin/mh2c request \
  --url https://nghttp2.org/httpbin/headers \
  --header 'user-agent:mh2c-go'
```

### POST Example

```sh
./bin/mh2c request \
  --url https://nghttp2.org/httpbin/post \
  --method POST \
  --header 'content-type: application/json' \
  --data '{"message":"hello from mh2c-go"}'
```

### Ping Example

```sh
./bin/mh2c ping \
  --host nghttp2.org \
  --ping-data mh2cping
```

### Observe Example

```sh
./bin/mh2c observe \
  --host nghttp2.org \
  --frame-filter headers \
  --frame-filter data \
  --stream-filter 1 \
  --save-body ./body.bin \
  --save-headers ./headers.txt
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
type = "sleep"
duration_ms = 500

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
./bin/mh2c script run --script-file ./examples/request.toml
```

Or use the checked-in examples directly:

```sh
./bin/mh2c script run --script-file ./examples/request.toml
./bin/mh2c script run --script-file ./examples/unusual-raw-sequence.toml
./bin/mh2c observe --host nghttp2.org --output jsonl | go run ./examples/jsonl-summary
./bin/mh2c script describe --type headers
./bin/mh2c script template request
./bin/mh2c script validate --script-file ./examples/request.toml
```

### Notes

- `make build-cli` creates `./bin/mh2c` for local use
- `make install` runs `go install ./cmd/mh2c`
- `--url` overrides `--scheme`, `--host`, `--port`, and `--path`
- `--body-file path/to/file` reads the request body from a file
- `--body-file -` reads the request body from stdin
- `--authority` overrides the `:authority` pseudo-header
- `mh2c request`, `mh2c ping`, `mh2c observe`, and `mh2c script ...` are the supported command forms
- `mh2c observe` performs the HTTP/2 handshake and continues printing received frames until `GOAWAY`, `--timeout`, or `--max-recv`
- `--max-recv N` limits the number of received frames in observe mode; `0` means unlimited
- `--stream-filter id` keeps stream-specific output focused on one stream while still showing connection-level frames
- `--direction-filter sent|received` is repeatable and keeps output focused on sent events, received events, or both
- `--frame-filter name` is repeatable and accepts `settings`, `headers`, `continuation`, `data`, `ping`, `goaway`, `window_update`, `rst_stream`, `push_promise`, `priority`, and `raw`
- `--output jsonl` emits one JSON line per event instead of the default text output
- `--data-format text|hex|both` controls how DATA, PING, and GOAWAY debug payloads are rendered
- `--data-limit N` truncates displayed payload bytes; `0` means unlimited
- `--decode-headers=false` disables HPACK header decoding in CLI output
- `--show-header-block=false` hides HPACK/header block fragments from the output
- `--save-output path` mirrors the displayed CLI output into a file
- `--save-body path` stores the captured response body in request/observe mode
- `--save-headers path` stores decoded response headers in request/observe mode
- `mh2c script run --script-file file.toml` executes a scripted frame sequence
- `mh2c script describe [--type action_type]` prints supported script action fields
- `mh2c script template request` prints a starter TOML script
- `mh2c script validate --script-file file.toml` checks a script without connecting to a server
- checked-in examples live under `examples/`
- the default request/script helpers aim to keep common HTTP/2 state in sync so normal debugging stays practical
- this does not turn `mh2c-go` into a high-level client: frames are still explicit and visible in the CLI output
- when you want to bypass helper-managed state and send unusual or intentionally invalid bytes, prefer `block_hex` or `raw`
- script mode does not auto-send connection preface or SETTINGS; include them explicitly when needed
- supported script actions are `preface`, `sleep`, `settings`, `headers`, `continuation`,
  `data`, `ping`, `goaway`, `window_update`, `rst_stream`, `priority`,
  `push_promise`, `raw`, and `receive`
- `sleep` uses `duration_ms = <int>` and prints progress such as `>> SLEEP 500ms`
- the script parser accepts the TOML subset used in the example above:
  strings, integers, booleans, string arrays, `[connection]`, and `[[action]]`
- received frames are printed with payload details such as decoded headers,
  DATA text/hex, SETTINGS entries, and PING payloads
- `go run ./cmd/mh2c request ...` still works for ad-hoc execution without producing a local binary

## Package Layout

```text
cmd/mh2c     manual HTTP/2 debugging CLI
client/      connection preface, raw I/O, frame send/receive, HPACK helpers
frame/       manual HTTP/2 frame types and binary encoding/decoding
hpack/       HPACK implementation and wrapper helpers
tlsconn/     TLS connection bootstrap with ALPN h2
internal/    internal wire helpers
```
