# Examples

Use `./bin/mh2c` after `make build-cli` for the common local workflow.

## Files

- `request.toml`: mixed send/receive script flow for a normal request-style exchange
- `observe-filtered.sh`: observe mode examples with frame filters, stream filters, and save helpers
- `unusual-raw-sequence.toml`: sends an unknown raw frame before a normal request to keep raw protocol details visible
- `window-update-repeat.toml`: uses `repeat` to send repeated WINDOW_UPDATE frames
- `jsonl-summary/`: small helper that reads `--output jsonl` and prints a compact summary

## Typical Runs

```sh
./bin/mh2c script run --script-file ./examples/request.toml
./bin/mh2c script run --script-file ./examples/unusual-raw-sequence.toml
./bin/mh2c script run --script-file ./examples/window-update-repeat.toml
./bin/mh2c observe --host nghttp2.org --frame-filter headers --frame-filter data --save-body ./body.bin
./bin/mh2c observe --host nghttp2.org --output jsonl | go run ./examples/jsonl-summary
./bin/mh2c script describe --type headers
./bin/mh2c script validate --script-file ./examples/request.toml
```
