#!/usr/bin/env bash
set -euo pipefail

arch="${1:?usage: check-native.sh amd64|arm64}"
machine="$(uname -m)"
case "${arch}:${machine}" in
    amd64:x86_64|arm64:aarch64|arm64:arm64)
        echo "Native build: ${arch} on ${machine}"
        ;;
    *)
        echo "Runner architecture ${machine} does not match ${arch}; emulation is not supported" >&2
        exit 1
        ;;
esac
