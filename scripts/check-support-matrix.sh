#!/usr/bin/env sh
set -eu

# Keep the JSON support contract and its rendered documentation page valid.
go test ./internal/osiris -run '^(TestSupportMatrixSchema|TestSupportMatrixDocumentationPage)$' -count=1

# The release gate is intentionally opt-in while backend closure tasks are in
# progress. CI release validation sets this variable and therefore fails on
# the first non-stable feature/backend cell rather than allowing status prose
# to conceal unfinished implementation evidence.
if [ "${OSIRIS_REQUIRE_STABLE_MATRIX:-0}" = "1" ]; then
	go test ./internal/osiris -run '^TestSupportMatrixStableReleaseGate$' -count=1
fi
