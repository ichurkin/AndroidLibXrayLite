#!/bin/bash
gomobile init

go mod tidy -v

gomobile bind -v \
  -androidapi 21 \
  -ldflags="\
    -s -w \
    -linkmode=external \
    -extldflags '-Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384' \
  " \
  ./
