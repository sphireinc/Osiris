#!/bin/bash

"$(cd "$(dirname "$0")" && pwd)/scripts/check-go-version.sh"
go build -o osiris ./cmd/osiris
