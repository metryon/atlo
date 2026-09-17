# Security and operating boundaries

This page describes atlo's current protections and limitations. For supported
versions and private vulnerability reporting, see the [security policy](../SECURITY.md).

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
- Release checksums detect mismatched/corrupted downloads; the build process
  does not produce signed provenance.
  Staging protects against compilation failures, but final promotion is atomic
  per file, not per bundle. Verify checksums after interruption and do not run
  concurrent release publishers against the same output directory.
- Cursor checks detect immediate repeats and unusable continuations; the CLI is
  stateless across invocations and cannot detect longer cursor cycles for an agent.

## Verification

CI runs tests with race detection, vet, Staticcheck, dependency verification,
and Go vulnerability scanning. Local regression tests and bounded fuzzing cover
input parsing, Markdown conversion, credentials in diagnostics, pagination,
preflight guards, and HTTP/upload behavior. See
[development and verification](development.md) for reproducible checks.

These checks and development reviews are not an independent security audit or a
guarantee that vulnerabilities are absent. Vulnerability scans depend on the
advisory database and code paths analyzed at the time of the scan. Cross-compiling
an executable does not verify its behavior on the target operating system.
