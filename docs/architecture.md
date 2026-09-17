# Architecture

atlo is a short-lived Go CLI. An agent chooses the task; atlo validates explicit
arguments, calls Jira or Confluence, and returns compact JSON. It has no embedded
LLM, shell execution of remote content, background service, or credential store.

## Responsibilities

| Area | Responsibility |
| --- | --- |
| `cmd/atlo` | Process entrypoint, interrupt handling, environment and standard streams. |
| `internal/cli/registry.go` | Command names, argument metadata, defaults, and discovery. |
| `internal/cli/parse.go` | Flags, JSON input, types, IDs, and mutually exclusive input sources. |
| `internal/cli/request.go` | Construct product-specific relative paths and payloads. |
| `internal/cli/content.go`, `markdown.go`, `adf.go` | Body loading, bounded Markdown trees, Confluence XHTML, and Jira ADF. |
| `internal/cli/execute.go`, `preflight.go` | Coordinate execution, own upload descriptors, and check edit guards. |
| `internal/cli/response.go` | Validate response shapes before reporting success or projecting records. |
| `internal/cli/format.go`, `projection.go` | Compact records and apply `--select`. |
| `internal/cli/pagination.go` | Interpret product-specific completion metadata and extract advancing cursors. |
| `internal/api/config.go` | Resolve separate product sites, credential aliases, and scoped-token routing. |
| `internal/api/client.go`, `response.go` | Authenticated HTTP, deadlines, response limits, and bounded read retries. |
| `internal/api/links.go` | Parse continuation headers without following their URLs. |
| `internal/api/errors.go` | Stable machine errors and structured credential redaction. |
| `internal/api/upload.go` | Stream multipart attachments without buffering file contents. |
| `internal/jsonx` | JSON byte/depth limits, UTF-8 checks, and unambiguous input decoding. |
| `internal/fileio` | Open regular files and reject special files without blocking on Unix. |

## Execution and ownership

1. Resolve a registered command and parse its input. Exact schema discovery and
   version output are local and need no credentials.
2. Build the request. Upload preparation opens one regular file and retains its
   descriptor; dispatch closes it on success, failure, and dry runs.
3. A dry run returns local request metadata. Other operations load the selected
   product's configuration and perform applicable preflight checks.
4. The HTTP client returns an `api.Response` containing both data and continuation
   information. Pagination is not mutable state on the client.
5. Validate the response, simplify records, and apply an optional projection.
   Errors go to stderr with a nonzero exit; success goes to stdout.

The operation layer depends on a small `productClient` interface. Tests can supply
transport behavior without a real site. Independent requests can share an API
client once its configuration is fixed; callers must not mutate its exported
configuration or HTTP client while requests run. An upload descriptor belongs to
one operation and should not be shared or modified during an upload.

## Applying SOLID principles

Responsibilities are separated by behavior rather than by artificial class
hierarchies. Parsing does not make HTTP calls, the transport does not know Jira
field semantics, and presentation does not decide which operations are permitted.
Dependency inversion is used at the transport boundary, where alternate behavior
is useful for tests. Native API fields remain extensible JSON objects rather than
an inheritance tree for every Jira field type.

Adding a command generally requires a registry entry, a request-building case,
and any relevant response validation/projection. Add a preflight only when an
actual consistency or workflow constraint requires it. Reuse the transport for
authentication, redirects, retries, and limits. Do not implement these separately
for each endpoint or add a second source of command schemas.

## Performance

The default reads remain one bounded page, with bodies and raw records opt-in.
Uploads stream an open file between small multipart framing buffers. Server error
reads stop after 4,097 bytes, enough to decide whether the 4 KiB diagnostic limit
was exceeded.

Jira issue updates and comment creation now go directly to their write endpoint.
The previous existence-only GET added a round trip without supplying a version
guard; Jira already validates existence and permissions on the write. Timestamp,
page-version, and workflow preflights remain in place where they carry actual
decision information.

Two local hotspots were also changed during the 2026-09-17 review:

- ADF text projection now writes through one shared string builder instead of
  copying each subtree's complete text into its parent.
- Command discovery considers only the depth of registered command names,
  rather than repeatedly joining every possible prefix of all arguments.

Measured on an Apple M4, macOS ARM64, Go 1.27.1; medians of three benchmark runs:

| Benchmark | Before | After | Allocation before → after |
| --- | ---: | ---: | ---: |
| Comment projection, 1,000 lines inside 30 nested containers | 193,855 ns/op | 46,020 ns/op | 1,734,045 → 210,320 B/op |
| Command lookup with 300 excess arguments | 326,328 ns/op | 2,692 ns/op | 64,136 → 10,269 B/op |

The first case improved about 4.2× and reduced allocated bytes about 88%. The
second is an intentionally adverse argument-validation workload. These are
microbenchmarks, not claims about end-to-end Jira or Confluence latency. Reproduce
them with `make bench`; results depend on the machine and runtime.

## Second-pass improvements

Comment reads with `--select id` now skip ADF text flattening. Full reads and
selections that include `text` retain the original representation. This avoids
unneeded local work; Jira still sends the comment body over the network. Projection
paths are split once per result set instead of once per record.

Using the same machine/runtime and medians of three runs:

| Benchmark | Before second pass | After second pass | Allocation before → after |
| --- | ---: | ---: | ---: |
| Select IDs from 100 comments, 100 lines each | 407,474 ns/op | 25,821 ns/op | 1,194,032 → 75,664 B/op |
| Select three fields from 100 records | 14,157 ns/op | 10,623 ns/op | 41,840 → 35,584 B/op |

The ID-only comment benchmark is about 16× faster with about 94% fewer allocated
bytes. These measurements cover formatting/projection, not HTTP or JSON decoding.

Numeric argument validation now consumes the registry's minimum/maximum values
directly. JSON numeric strings are rejected; normal CLI numeric flags are converted
before validation. Validation order is stable when several properties are wrong.

Pagination handles multiple `Link` header fields, reordered parameters, quoted
commas, and relation lists. It checks exact relation names rather than prefixes.
See [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288.html) for the header format.
The operation layer then extracts a cursor and rejects missing, conflicting, or
immediately repeating continuations. It never follows a header's hostname.

## Final-pass guard and pagination cleanup

Pagination now lives separately from record formatting, with dedicated helpers
for Jira offset pages, Jira search tokens, and Confluence links. For offset pages,
an explicit `isLast` controls continuation; otherwise a numeric `total` takes
precedence over the page-size fallback. The fallback uses the returned
`maxResults` when available. This accounts for server caps below the requested
limit, as described in [Jira's pagination contract](https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/#pagination).
Returned offsets must match the request, and malformed metadata produces an error.

Preflight reads share one transport step, followed by command-specific checks.
They validate target identity and guard metadata before deciding whether a write
can proceed. Required transition fields are checked in a deterministic order;
Jira retains responsibility for field-value semantics. Integer checks preserve
`json.Number` precision and reject strings and overflow instead of converting
arbitrary values through formatted text. No new dependency or command flag was
needed for these changes.
