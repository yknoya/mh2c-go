# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go module for manual HTTP/2 debugging. Keep changes close
to the package they affect.

- `cmd/mh2c/`: CLI entrypoint, script-mode parser, and CLI tests
- `client/`: connection preface, frame send/receive, HPACK helpers, integration tests
- `frame/`: HTTP/2 frame types, binary encoding/decoding, frame unit tests
- `hpack/`: thin wrappers around `golang.org/x/net/http2/hpack`; do not vendor or copy HPACK implementation code here
- `tlsconn/`: TLS + ALPN `h2` bootstrap
- `internal/framefmt/`: reusable frame text formatting helpers shared by CLI output code
- `internal/wire/`: low-level wire helpers shared by packages

Tests live next to the code as `*_test.go`.

## Build, Test, and Development Commands

Use the `Makefile` for the common path, or run Go commands directly.

- `make build`: build all packages with `go build ./...`
- `make build-cli`: build the local CLI binary at `./bin/mh2c`
- `make install`: install `mh2c` with `go install ./cmd/mh2c`
- `make test`: run the full test suite with `go test ./...`
- `make fmt`: format all Go packages with `go fmt ./...`
- `./bin/mh2c --help`: inspect CLI options locally after `make build-cli`
- `./bin/mh2c script run --script-file ./request.toml`: run a scripted frame sequence
- `go run ./cmd/mh2c ...`: use for ad-hoc execution when you do not want to produce a local binary

Run `make fmt` and `make test` before opening a PR.

## Coding Style & Naming Conventions

Follow standard Go formatting and keep files `gofmt`-clean. Use tabs as Go
formats them. Package names stay short and lowercase (`frame`, `client`,
`tlsconn`). Exported identifiers use `CamelCase`; unexported helpers use
`camelCase`.

Prefer small, protocol-focused changes. This project is a manual HTTP/2 client,
not a high-level convenience client, so preserve frame-level visibility and
explicit control in the CLI.

Keep `frame.String()` as a concise semantic summary for the frame itself. Put
formatting that depends on output settings, truncation, decoded headers, or
warnings in `internal/framefmt` or the CLI output layer; keep HPACK state
orchestration out of formatter packages.

Typed frame structs should store header metadata in a `FrameHeader Header`
field rather than duplicating `StreamID` or `Flags` fields. Constructors such as
`NewXxxFrame` should create complete ordinary headers, including `Type` and
payload-derived `Length`, while parsed frames should preserve the received wire
header. Keep deliberately malformed or exact-length frames on the raw path
(`RawFrameFromExactParts`, `SendRawFrameExact`, and script `raw.length`).

Do not copy or vendor third-party implementation code into this repository.
When protocol helpers are needed, use Go module dependencies and keep only thin
local wrappers or adapters here. In particular, do not add files with external
copyright headers such as `Copyright 20XX The Go Authors`.

## Testing Guidelines

Use the standard `testing` package. Add focused unit tests in the touched
package and add integration coverage when behavior crosses package boundaries,
for example `client/integration_test.go`.

Name tests by behavior, such as `TestPushPromiseFrameRoundTrip` or
`TestHTTP2RoundTripAgainstTLSServer`.

## Subagent Workflow

When using Codex subagents in this repository, keep delegation scoped to the
manual HTTP/2 debugging purpose.

- Use explorer-style agents for read-only discovery, such as finding the frame
  path, CLI output policy, or test coverage gaps.
- Use worker-style agents only with explicit file or package ownership, and
  require them to preserve frame-level visibility, low-level control, and raw
  escape hatches.
- Use review-style agents after implementation to prioritize protocol
  correctness, observability regressions, missing tests, and accidental
  high-level HTTP client behavior.
- Do not ask subagents to vendor HPACK code, hide raw frame bytes behind
  convenience APIs, or revert edits made by other agents.

## Commit & Pull Request Guidelines

Commits currently follow concise Conventional Commit style, e.g.
`feat: add manual http2 cli scripting and integration coverage`.

PRs should include:

- a short summary of the protocol or CLI behavior changed
- the verification commands you ran, usually `make fmt` and `make test`
- sample frame output only when it helps explain a debugging-oriented change

Screenshots are usually unnecessary for this repository.
