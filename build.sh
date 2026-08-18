#!/bin/bash -xe

#CGO_ENABLED=0
# Stamp the current git commit into the binary so `shuttle-go --version` reports it.
# Keep the committed VERSION file in sync too (used by the Nix build, which has no .git).
VERSION="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
printf '%s\n' "${VERSION}" > VERSION
GOOS=linux GOARCH=amd64 go build -v -ldflags "-X main.version=${VERSION}" -o shuttle-go
