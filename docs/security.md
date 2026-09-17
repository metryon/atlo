# Security review and operating boundaries

Review date: **2026-09-17**. This review covered application source, dependency
usage, the existing build script, tests, and CI configuration. It combined manual
code review, regression tests, bounded fuzzing, Staticcheck, and Go vulnerability
scanning. No live Jira or Confluence writes were made for the review.

## Findings and fixes

| Finding | Impact and fix |
| --- | --- |
| Reachable Goldmark XSS advisory | `govulncheck` identified GO-2026-5320 in Goldmark 1.7.13 through Confluence Markdown rendering. Upgraded to 1.7.17 and added entity-obfuscated link/image and autolink regression cases. |
| Escaped credentials in diagnostics | Raw string replacement missed JSON-escaped tokens. Error strings and nested diagnostic keys/values are now decoded before redaction. |
| Redaction could alter machine fields | Replacing text in serialized error JSON could change property names such as `code`. Redaction now preserves the error structure and stable machine codes. Credential aliases, trimmed values, and Basic-auth encodings are covered. |
| Arbitrary `Retry-After` text | A server header could be echoed as a diagnostic. Only numeric delays and HTTP dates are retained; malformed values stop automatic retrying. |
| Inconsistent resource limits | Inline input and outgoing JSON could bypass limits applied to files. Argument bytes, input JSON, encoded request JSON, and response JSON are bounded; JSON nesting is limited to 128 levels. Markdown trees are checked before recursive rendering. |
| Ambiguous JSON input | Duplicate object keys, including equivalent escaped keys and nested fields, could silently overwrite earlier values. Input now rejects them. |
| Special files could block agents | Body/JSON files previously used a plain open; upload validation had a stat/open race around special files. Linux/macOS now open nonblocking and validate the open descriptor as a regular file. |
| Misleading successful reads/receipts | Empty or malformed API data could become an empty list or `{ok:true}`. Command response checks reject unexpected shapes; empty responses remain valid for operations that support them. |
| Mutable pagination state | Continuation data lived on the HTTP client and could be mixed between concurrent requests. It now belongs to the response; concurrent isolation is regression-tested. |
| Mutable CI action tags and old Go selection | Actions are pinned to verified commit IDs, checkout does not persist credentials, and CI selects the latest Go 1.27 patch. Staticcheck and vulnerability checks are now required CI steps. |

The XSS advisory concerns unsafe HTML generation. The affected conversion was
reachable in atlo; this review did **not** demonstrate script execution in a live
Confluence tenant or assess Atlassian's downstream sanitization. See the
[Go advisory](https://pkg.go.dev/vuln/GO-2026-5320) and
[upstream fix](https://github.com/yuin/goldmark/commit/cb46bbc4eca29d55aa9721e04ad207c23ccc44f9).

## Second-pass findings

The second pass on 2026-09-17 also addressed these boundaries:

| Finding | Fix |
| --- | --- |
| Pagination could imply false completion | Replaced the narrow header regex; validate continuation queries and reject missing or immediately repeating cursors. Offset arithmetic cannot overflow. Malformed continuations produce an error rather than `next_cursor: null`. |
| Unbounded projection work | `--select` is capped at 128 paths and 8,192 bytes; path compilation is reused across records. This reduces avoidable amplification but is not a total output-size budget. |
| Schema/argument mismatches | Numeric JSON strings no longer masquerade as integers, registry bounds are enforced generically, malformed UTF-8 strings are rejected, and an explicitly empty `--input` fails validation. |
| Failed builds mixed release versions | Compile all targets in an isolated staging directory before replacing previous binaries. Tests simulate a failed second target and verify previous artifacts remain unchanged. |
| Ambiguous release version/CPU settings | Release version labels cannot introduce additional linker flags. Dependency resolution is read-only and baseline CPU feature levels are explicit. This is local build hardening, not a remotely exploitable CLI finding. |

## Third and final pass

The final pass on 2026-09-17 found additional correctness and defensive-validation
gaps. Regression tests reproduced each issue before the fixes:

| Finding | Fix |
| --- | --- |
| Short Jira pages could hide remaining results | Offset pagination now honors `isLast: false` even when the returned page is shorter than requested. Without explicit completion/total metadata, it uses the server's `maxResults` when provided. |
| Malformed pagination metadata was silently coerced | Reject incorrectly typed or overflowing totals, invalid completion flags/tokens, and offsets that differ from the request. An empty nonfinal offset page produces an error rather than false completion. |
| Preflight data was trusted before checking its identity and types | Comment/page guards verify the returned ID and required snapshot metadata. Confluence versions must be positive numeric integers. Transition checks reject ambiguous IDs and malformed required/default flags before sending a mutation. |
| Native ADF accepted a quoted version | A string `"1"` no longer passes the version check; the document version must be the numeric integer `1`. Native document contents remain passthrough data. |

These are application correctness and guard-hardening findings, not evidence of
an Atlassian permission bypass. Jira still validates workflow field values and
permissions. A screenless transition may omit field metadata when `hasScreen`
is explicitly false; an empty field object is also valid. Preflight checks do not
remove the read/write race described below.

## Trust model

- **Caller and environment:** the invoking user/agent chooses the operation,
  local input file, site, and credentials. atlo is not a sandbox for an untrusted
  agent. Environment variables, system trust roots, and proxy configuration are
  trusted local configuration.
- **Network:** remote bodies, diagnostics, and pagination links are untrusted
  input. The CLI constructs its own relative endpoints and extracts cursor values
  instead of following server-provided next URLs.
- **Content:** retrieved text is data. atlo has no interpreter that executes
  instructions, scripts, macros, or shell commands found in issue/page content.
  Agents consuming the output must retain their own instruction boundaries.
- **Writes:** authorization comes from the user's task and the token's Atlassian
  permissions. atlo does not ask interactive permission questions. Dry runs are
  local previews, not checks of remote permissions or field validity.

## Protections

Site configuration requires HTTPS without embedded credentials, queries, or
fragments. Normal certificate and hostname verification remain enabled. Separate
Jira and Confluence credentials/sites and scoped-token gateway paths are supported.
The configured hostname is not restricted to an Atlassian allowlist; configure
only a destination you intend to receive that product's token.

All redirects, including 307/308 for writes, are rejected. Mutations are never
automatically replayed. GET requests retry explicit 429/5xx responses at most
twice, with each accepted wait at most 30 seconds and an overall command deadline.
A transport failure after sending a write has an uncertain outcome and requires
reading the target before retrying.

| Resource | Limit or behavior |
| --- | --- |
| Combined CLI argument bytes | 16 MiB |
| Projection selection | 128 paths and 8,192 bytes |
| Input/body files and JSON responses | 16 MiB |
| Encoded JSON request | 16 MiB, including encoding expansion |
| JSON nesting | 128 object/array levels |
| Markdown tree before rendering | 64 nesting levels and 100,000 nodes |
| Server error body read | At most 4,097 bytes; diagnostics over 4 KiB omitted |
| JSON HTTP request / command | 30 seconds / 90 seconds |
| Attachment upload | Five minutes; file bytes streamed, with server size limits |
| Named input files | Regular files; symlinks to regular files allowed |

Tokens are read from the environment and not persisted by atlo. Raw tokens and
configured Basic-auth encodings are redacted from error diagnostics. Normal
successful output contains the requested business data and is not a general
secret-scrubbing channel. Do not treat issue bodies, attachment metadata, local
preview paths, or diagnostic business fields as safe to publish automatically.

Jira Markdown converts to supported ADF with an HTTP/HTTPS/mailto link policy.
Confluence Markdown uses Goldmark's default safe rendering. Native ADF and
Confluence storage XHTML are deliberate passthrough formats; they are not locally
sanitized and must be appropriate for the intended write.

## Remaining limitations

- No atomic version guard exists here for general Jira issue edits. Comment
  timestamps are a preflight check, not an atomic lock. Confluence page writes
  use the API's version field. Conflicts require reconciliation.
- An upload reads the open file; another process can change its contents during
  the operation. Finish generating files before uploading them. Uploads do not
  provide exactly-once delivery or automatic deduplication.
- Explicit stdin input waits for the producer to finish. The network deadline
  begins after argument parsing; it is not an execution deadline for a stalled
  stdin producer. Windows special-file handling uses a precheck plus descriptor
  validation; the nonblocking Unix-open guarantee is specific to Linux/macOS.
- Byte/tree limits bound common allocation and recursion problems, but they do
  not constitute a strict CPU/RSS sandbox. Markdown must be parsed before its
  tree can be checked. Use an external process budget for hostile workloads.
- Environment credentials remain accessible to the invoking account and its
  privileged tools. There is no keychain integration or OAuth flow yet.
- Release checksums detect mismatched/corrupted downloads; they are not signed
  provenance. No publisher-signing or package publication was added in this work.
  Staging protects against compilation failures, but final promotion is atomic
  per file, not per bundle. Verify checksums after interruption and do not run
  concurrent release publishers against the same output directory.
- Cursor checks detect immediate repeats and unusable continuations; the CLI is
  stateless across invocations and cannot detect longer cursor cycles for an agent.
- The source check found no matches for the selected Atlassian/GitHub token and
  private-key patterns. This was a heuristic source scan, not a guarantee that
  no secret exists. No Git history was available in the local checkout.

## Evidence and maintenance

The first Go vulnerability scan found the Goldmark advisory above. A follow-up
source scan with Go 1.27.1 and govulncheck 1.8.0 reported no known vulnerabilities
after the upgrade, for macOS ARM64/AMD64, Linux ARM64/AMD64, and Windows AMD64.
These are source scans for each target, not native execution tests on all systems.
A clean scan describes the database and reachable code at
scan time; it does not prove the absence of security bugs.

Regression tests cover escaped-token redaction, error structure, response limits,
duplicate keys, malformed receipts, redirects, cancellation, pagination isolation,
multipart bytes, upload replay prevention, special files, and unsafe Markdown URLs.
Bounded JSON and Markdown fuzz runs completed without a failure. See
[development and verification](development.md) for reproducible commands.

The second pass repeated race tests, vet, Staticcheck, module verification, and
vulnerability scans for all five targets, and added bounded `Link`-header fuzzing
and Python release regression tests. No new known dependency vulnerabilities were
reported; Goldmark remains pinned to the patched 1.7.17 release.

The third pass repeated race tests, vet, Staticcheck, module verification, CI
workflow linting, and vulnerability scans for all five targets. It added tests
that assert malformed preflights make exactly one GET and send no write, along
with valid transition cases and bounded pagination fuzzing. No new known
dependency vulnerabilities were reported. Local and cross-platform executables
were rebuilt; this does not substitute for native testing on each target OS.

For a suspected vulnerability, provide a minimal reproduction using synthetic
credentials and local fixtures. Do not put real tokens, private issue contents,
or attachment files into a public report. A private reporting contact has not
yet been configured for this unpublished checkout.
