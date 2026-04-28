#!/bin/sh

set -eu

./bin/mh2c observe \
  --host nghttp2.org \
  --frame-filter headers \
  --frame-filter data \
  --stream-filter 1 \
  --save-body ./body.bin \
  --save-headers ./headers.txt

./bin/mh2c observe \
  --host nghttp2.org \
  --output jsonl \
  --frame-filter ping \
  --frame-filter goaway \
  --data-format hex
