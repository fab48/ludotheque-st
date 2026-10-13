#!/bin/bash
# Compile le lanceur pour Linux et Windows (Go 1.24+, sans CGO).
set -e
cd "$(dirname "$0")/sources/lanceur"
export CGO_ENABLED=0 GOFLAGS=-mod=vendor
GOOS=linux   go build -ldflags "-s -w" -o ../../ludotheque .
GOOS=windows go build -ldflags "-s -w -H windowsgui" -o ../../Ludotheque.exe .
echo "Compilation OK : ludotheque et Ludotheque.exe"
