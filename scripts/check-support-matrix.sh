#!/usr/bin/env sh
set -eu

# Keep the JSON support contract and its rendered documentation page valid.
go test ./internal/osiris -run '^(TestSupportMatrixSchema|TestSupportMatrixDocumentationPage)$' -count=1
