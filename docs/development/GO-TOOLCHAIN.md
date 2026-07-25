# Go toolchain policy

Osiris supports the Go 1.26 release line. The `go 1.26` directive in the root
`go.mod` is the single source of truth for this requirement. CI reads that
directive through `actions/setup-go`, and local builds run
`scripts/check-go-version.sh` before compiling.

## Developer setup

Install a current Go 1.26.x release, then verify it from the repository root:

```sh
go version
./scripts/check-go-version.sh
go test ./...
./build.sh
```

The version check accepts released 1.26 patch versions only. It intentionally
rejects older release lines, future release lines, and prereleases so a build
does not silently depend on a different standard library or compiler.

## Upgrade policy

Changing the supported Go release line requires one reviewable change that:

1. updates the `go` directive and this policy together;
2. verifies CI installs the same release line;
3. runs the complete test suite on the new minimum release; and
4. records whether source changes use new language or standard-library APIs.

Do not use the `toolchain` directive to trigger an implicit download during a
normal build. Contributors and CI should install the release line explicitly.
