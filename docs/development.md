# Development and verification

Source compatibility starts at Go 1.25. Build distributed executables with a
currently supported, patched Go release; CI currently selects Go 1.27.x with
`check-latest`. Go's standard library is included in the binary, so replacing a
Go installation alone does not patch an already-built atlo executable.

## Local checks

```sh
make test
make audit
make bench
```

`make test` runs race-enabled tests, `go vet`, and Python release-script regression
tests with a fake compiler. `make audit` verifies dependency
checksums and runs pinned Staticcheck 0.8.1 and govulncheck 1.8.0 tools. Tool
installation and vulnerability checks require network access; tool dependencies
are not added to atlo's runtime module dependencies.

Run bounded fuzz checks separately:

```sh
go test ./internal/jsonx -run '^$' -fuzz '^FuzzDecode$' -fuzztime=15s -parallel=2
go test ./internal/cli -run '^$' -fuzz '^FuzzMarkdownADF$' -fuzztime=15s -parallel=2
go test ./internal/api -run '^$' -fuzz '^FuzzLinkHeaders$' -fuzztime=15s -parallel=2
go test ./internal/cli -run '^$' -fuzz '^FuzzPagination$' -fuzztime=15s -parallel=2
```

Tests use synthetic credentials and mock transports. They do not require an
Atlassian account or mutate a real site. Race detection includes concurrent
response pagination. Unix file tests exercise a named pipe without a writer.
Fuzzing checks local input/rendering boundaries; it is not an Atlassian API test.
Pagination fuzzing also exercises malformed remote metadata. Guard regression
tests assert that rejected preflights send no mutation, and cover neighboring
valid transitions, including required fields with defaults and false/zero values.

For CI workflow edits, also run `actionlint .github/workflows/*.yml`. Keep action
commit pins and tool versions current through reviewed updates. Never embed
Atlassian tokens into CI tests or examples.

## Build and existing release outputs

```sh
make build VERSION=0.1.0-dev
make release VERSION=v0.1.0
```

Override `GO=/path/to/go` when Go is not on PATH. The release script
cross-compiles standalone binaries with `CGO_ENABLED=0` and writes checksums to
`bin/release/SHA256SUMS`:

| Target | Output |
| --- | --- |
| macOS Apple Silicon | `atlo-darwin-arm64` |
| macOS Intel | `atlo-darwin-amd64` |
| Linux ARM64 | `atlo-linux-arm64` |
| Linux x86-64 | `atlo-linux-amd64` |
| Windows x86-64 | `atlo-windows-amd64.exe` |

Each target also has a versioned `.tar.gz` archive (`.zip` on Windows) containing
the executable, README, MIT license, and third-party notices. The checksum manifest
covers raw binaries, archives, and the accompanying documents.

Release compilation uses an isolated staging directory and `-mod=readonly`.
If any target fails to compile, the previous binaries and checksums remain intact.
After all targets succeed, files are replaced individually and the checksum
manifest is replaced last. Verify the manifest after an interrupted promotion;
do not run concurrent publishers into the same directory. Version labels are
limited to 128 letters/digits and `.`, `_`, `+`, `-`, starting with a letter or
digit. CPU targets explicitly use AMD64 v1 and ARM64 v8.0 baselines.

Linux outputs are built without a glibc dependency. Distributions such as Ubuntu
and Arch use the same binary for a given CPU architecture. Cross-compilation
confirms the build, not runtime behavior on every target. Native smoke tests on
the intended systems remain useful before publishing a release.

The [release workflow](../.github/workflows/release.yml) publishes tagged releases
after native smoke tests and Arch package verification. Homebrew and AUR recipe
templates are generated against the release checksums. See
[installation and distribution](installation.md) for publishing, manual package
index updates, and the pending AUR account requirement. Ubuntu can use the Bash
installer; no `.deb` package is currently produced.

## Reviewing changes

Preserve command names, JSON envelopes, exit codes, and economical defaults.
If validation becomes stricter, document the rejected inputs and test both the
rejection and neighboring valid cases. Test externally observable behavior at
the HTTP boundary: paths, methods, headers, multipart bytes, receipts, guards,
and retry counts.

Add dependencies only when their benefit warrants the additional maintenance and
security surface. Goldmark is the only direct runtime dependency. Run vulnerability
checks after dependency changes and rebuild artifacts after security fixes.
