#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only

: "${OUTDIR:=_build}"

mkdir -p "$OUTDIR"
CGO_ENABLED=1 go build \
    -trimpath -buildvcs=false -ldflags="-s -w" \
    -o "$OUTDIR" ./...
