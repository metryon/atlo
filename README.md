# atlo

A small, agent-first CLI for **Jira Cloud and Confluence Cloud**. One Go binary,
API-token authentication, compact JSON, non-interactive commands. Agents provide
the reasoning; atlo provides predictable operations.

Repository: [metryon/atlo](https://github.com/metryon/atlo).

See [docs/](docs/README.md) for architecture, security boundaries, and development
and verification procedures.

## Build and run

Requires Go 1.25 or newer to build. The resulting executable needs no Go runtime.

```sh
go build -trimpath -o bin/atlo ./cmd/atlo
./bin/atlo version
./bin/atlo schema
```

Put the binary on your PATH if desired. Examples below use `atlo`.

## Authentication

Create an [Atlassian API token](https://id.atlassian.com/manage-profile/security/api-tokens).
Set these environment variables through your shell or secret manager:

```sh
export ATLASSIAN_URL="https://your-site.atlassian.net"
export ATLASSIAN_EMAIL="you@example.com"
# Populate ATLASSIAN_API_TOKEN from your secret manager or a hidden shell prompt.
# For example, in zsh:
read -rs 'ATLASSIAN_API_TOKEN?API token: '
export ATLASSIAN_API_TOKEN

atlo auth check --product jira
atlo auth check --product confluence
```

The CLI uses email + token Basic authentication over HTTPS. Tokens stay in the
environment: no token flags, config files, telemetry, or credential persistence.
It does not automatically load `.env` files.

For a **scoped API token**, also set `ATLASSIAN_CLOUD_ID`. Requests then go to
`https://api.atlassian.com/ex/jira/<cloud-id>` or
`https://api.atlassian.com/ex/confluence/<cloud-id>`. Use a token with the scopes
needed for your operations. See Atlassian's [API-token instructions](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/).

For separate sites or credentials, product settings override shared settings:

| Setting | Shared | Product override |
| --- | --- | --- |
| Site | `ATLASSIAN_URL` | `JIRA_URL`, `CONFLUENCE_URL` |
| Email | `ATLASSIAN_EMAIL` | `JIRA_EMAIL`, `CONFLUENCE_EMAIL` |
| Token | `ATLASSIAN_API_TOKEN` | `JIRA_API_TOKEN`, `CONFLUENCE_API_TOKEN` |
| Cloud ID | `ATLASSIAN_CLOUD_ID` | `JIRA_CLOUD_ID`, `CONFLUENCE_CLOUD_ID` |

`JIRA_USER`, `JIRA_TOKEN`, `CONFLUENCE_USER`, and `CONFLUENCE_TOKEN` are accepted
as legacy product-level aliases. Existing confit credentials therefore work.
Use product-specific cloud IDs when the two products are on different sites.

## Agent contract and token efficiency

- Success: exactly one compact JSON value on stdout, `{ "data": ..., "meta": ... }`.
- Failure: stdout is empty; stderr contains one `{ "error": ... }` JSON object.
- Nothing prompts, opens a browser, or needs a TTY. `--pretty` is opt-in.
- Reads return compact records. Avatars, expansion scaffolding, redundant links,
  search excerpts, and page bodies are omitted by default.
- Lists fetch **one page of 10 records** by default; `--limit` accepts 1–100.
- `meta.next_cursor` is a continuation token or `null` at the end. Continue with
  `--cursor` and the same filters. No command automatically fetches every page.
- `--fields` on Jira reads limits what the server returns. `--select` projects
  the returned data while preserving pagination metadata. Missing paths are `null`.
- `--raw` opts into native API response data; it retains pagination metadata.
- `schema` accepts a namespace or exact command, so an agent can discover only
  the contract it needs. It works without credentials or a network connection.

```sh
atlo schema jira
atlo schema jira issue update
atlo jira issue get ENG-123 --select key,summary,status
atlo jira issue search --jql 'project = ENG ORDER BY updated DESC' \
  --limit 5 --select key,summary,status
atlo confluence page get 12345                     # title, version, metadata
atlo confluence page get 12345 --body-format storage # explicit full body
```

Example search response (illustrative):

```json
{"data":[{"key":"ENG-123","summary":"Build atlo","status":"Open"}],"meta":{"count":1,"next_cursor":"opaque-token"}}
```

Follow-up:

```sh
atlo jira issue search --jql 'project = ENG ORDER BY updated DESC' \
  --limit 5 --select key,summary,status --cursor 'opaque-token'
```

For `--select`, paths are relative to each compact data record, for example
`key,assignee.accountId`. Nested paths become literal keys in the projected output.
With `--raw`, paths are relative to the native response object instead.

## Jira

```sh
atlo jira project list
atlo jira issue get ENG-123
atlo jira issue get ENG-123 --fields summary,description,customfield_10001
atlo jira issue search --jql 'assignee = currentUser() AND resolution = Unresolved'

# Discover project-specific issue types and field requirements before creating.
atlo jira project issue-types ENG
atlo jira project create-fields ENG --issue-type-id 10001
# Verify a known account before assigning a new issue.
atlo jira user assignable --project ENG --account-id ACCOUNT_ID
atlo jira issue create --input @examples/create-issue.json --dry-run
atlo jira issue create --input @examples/create-issue.json

# Discover edit metadata and change only the supplied fields.
atlo jira issue edit-fields ENG-123
atlo jira issue update ENG-123 --fields '{"summary":"Updated title"}'

# Discover the actual workflow; never guess a transition ID or resolution.
atlo jira issue transitions ENG-123
atlo jira issue transition ENG-123 --transition-id 31 --dry-run

atlo jira comment list ENG-123 --limit 5
atlo jira comment get ENG-123 123456
atlo jira comment add ENG-123 --body-file comment.md
```

Issue create/update accepts native Jira `fields`, including custom fields.
Rich-text fields such as descriptions use **Atlassian Document Format (ADF)**.
Comments interpret `--body` and `--body-file` as **Markdown by default** and convert
it to Jira ADF. Paragraphs, headings, bold, italics, strikethrough, links (including
bare URLs), lists, quotes, code blocks, and tables retain their formatting.
Blank-line runs separate paragraphs without inserting empty paragraphs. Soft line
wraps become spaces; Markdown hard breaks become line breaks. Task list checkboxes
are rendered as textual `[ ]` / `[x]` markers.

Use `--format text` for literal text without Markdown interpretation. Blank lines
still separate paragraphs, and individual newlines become line breaks. Native
`--adf` objects are passed through unchanged. Raw HTML, Markdown images, relative
links, and blocks nested in contexts Jira cannot represent return validation
errors instead of silently losing content. Use attachment-aware ADF for images.

To repair an existing comment, read it first and pass its exact `updated` value:

```sh
atlo jira comment get ENG-123 123456
atlo jira comment update ENG-123 123456 \
  --expected-updated 'TIMESTAMP_FROM_GET' --body-file comment.md --dry-run
```

Remove `--dry-run` to apply an authorized edit. Only the body is sent; the CLI
does not change visibility. The updated timestamp check catches stale snapshots
before writing, but Jira provides no atomic compare-and-swap here. Review shared
comments promptly and verify the result. A failed write is never auto-replayed.

Comment reads return a text projection; use `--raw` to retain formatting and ADF.
Before transitioning, atlo checks that the transition is available and that
required fields without defaults have explicit values. Jira still performs the
authoritative field validation.

## Confluence

```sh
atlo confluence space list --key ENG
atlo confluence page list --space-id 123
atlo confluence page search --cql 'type=page AND space=ENG ORDER BY lastmodified DESC'
atlo confluence page children 12345

atlo confluence page create --space-id 123 --title 'Design notes' \
  --parent-id 12345 --body-file notes.md --dry-run

# Read the version and body used to prepare your edit.
atlo confluence page get 12345 --body-format storage
atlo confluence page update 12345 --title 'Design notes' \
  --expected-version 7 --body-file notes.md

atlo confluence page delete 12345 --dry-run
atlo confluence page delete 12345 --yes
```

Markdown supports headings, tables, lists, fenced code, emphasis, links, and
images through Goldmark's GFM parser. Raw HTML in Markdown is omitted. Use
`--format storage` for exact Confluence XHTML, including macros; storage input is
sent unchanged. Image URLs are retained; local images are not uploaded.

Page updates replace the body. Pass the version obtained when reading the page
as `--expected-version`; atlo checks it and sends version + 1. Confluence rejects
a competing update that wins the race. A conflict is never silently retried.
To preserve existing macros, edit the storage body and use `--format storage`.
There is no lossless Markdown round trip for existing Confluence pages.

Deletion requires `--yes`, reads the page first, and only accepts current pages.
This version exposes moving pages to trash, not permanent purge.

## Attachments

```sh
# Preview locally; remove --dry-run to upload the file.
atlo jira attachment upload ENG-123 --file ./report.pdf --dry-run
atlo confluence attachment upload 12345 --file ./diagram.png --dry-run

# Optional uploaded filename and MIME type.
atlo jira attachment upload ENG-123 --file ./build/output.bin \
  --filename report.pdf --content-type application/pdf

# Confluence supports an attachment comment and optional notification suppression.
atlo confluence attachment upload 12345 --file ./report.pdf \
  --comment 'Validation results' --minor-edit

# Inspect attachments, including after an uncertain upload.
atlo jira issue get ENG-123 --fields attachment --select attachment
atlo confluence attachment list 12345 --limit 5
```

Each upload sends one regular file, streamed from disk, and returns a compact
array of attachment metadata. Jira returns `id`, `filename`, `size`, `mimeType`,
and `content` (download URL). Confluence returns `id`, `filename`, `size`,
`mimeType`, `version`, and `download` when supplied by the server; download links
may be relative to the configured Confluence site. `--raw` preserves the native
response. `--select id,filename,size` keeps receipts small.

`--file` requires a local file path, not stdin or a directory. The uploaded name
defaults to its basename; MIME type is inferred from the uploaded filename or
file bytes. Dry runs validate readability and show name, size, MIME type, and
form fields without including file contents or contacting either site.

Uploads have a five-minute deadline and follow each site's attachment size
limits; the 16 MiB JSON limit does not apply to file contents. They are never
automatically retried. After an uncertain result, inspect the target's attachments
before retrying to avoid duplicates. Confluence creates a new attachment and
rejects an existing filename instead of replacing it. Uploading attaches the file
to the issue/page; embedding an image or adding a link to its body is a separate
edit. Markdown local image paths are not uploaded automatically.

## JSON input and previews

Each command accepts the same argument names through flags or a JSON object:

```sh
atlo jira issue create --input @examples/create-issue.json
cat examples/create-issue.json | atlo jira issue create --input - --dry-run
cat notes.md | atlo confluence page create \
  --space-id 123 --title 'Notes' --body-file - --dry-run
```

Flags override JSON arguments. Do not use stdin for both `--input -` and
`--body-file -`. Unknown arguments, duplicate flags, invalid types, and
unexpected positional arguments produce structured validation errors. Duplicate
JSON object keys (including nested or escaped duplicates) are rejected. Numeric
arguments in JSON must be numbers, not quoted strings; numeric CLI flags keep
their usual syntax. An explicitly empty `--input` is an error.

`--select` accepts up to 128 paths and 8,192 bytes. Comment reads skip text
conversion when the selection excludes `text`. An unusable or immediately
repeating pagination cursor produces a structured error rather than implying
that all results have been read.

`--dry-run` builds the exact product, method, relative path, and request body
without network access or credentials. Its metadata says `validation: local_only`:
it does not check permissions, remote field requirements, page versions, or
workflow availability. It is available on every mutation, including deletion.

## Errors and request behavior

```json
{"error":{"code":"conflict","message":"Page version differs from expected-version; fetch and reconcile before retrying.","retryable":false,"details":{"expected":7,"actual":8}}}
```

| Exit | Meaning |
| --- | --- |
| 0 | Success |
| 1 | API, protocol, or unexpected failure |
| 2 | Invalid arguments or input |
| 3 | Configuration, authentication, or permissions |
| 4 | Not found |
| 5 | Conflict |
| 6 | Rate limited |
| 7 | Transport failure or cancellation |

JSON requests have a 30-second timeout and each command a 90-second deadline;
attachment uploads use five minutes. GET
requests retry 429/5xx at most twice; Retry-After is honored within a 30-second
wait budget. Longer delays are returned for the caller to schedule. Writes are
never replayed automatically: a transport failure can mean the server committed
the change, so read the target before retrying. Redirects are rejected. Input
and response JSON bodies are limited to 16 MiB, as are combined argument bytes
and encoded JSON requests. JSON nesting is limited to 128 levels; Markdown trees
are limited to 64 nesting levels and 100,000 nodes before rendering. Named input
files must be regular files. Credential values are redacted from structured
errors; server diagnostic reads are bounded to 4 KiB plus one overflow byte.
See [security and operating boundaries](docs/security.md) for protections and
limitations, and the [security policy](SECURITY.md) to report a vulnerability privately.

Jira issue edits have no general optimistic version guard in this implementation.
Use small field updates and re-read shared fields before changing them. API search
results can be eventually consistent.

## License

atlo is licensed under the [MIT License](LICENSE).
Copyright (c) 2026 Richard Diphoorn.

Commercial use, modification, and redistribution are permitted. Copies or
substantial portions must retain the copyright and license notices.

## Development

```sh
go test -race -cover ./...
go vet ./...
make audit
make build
make release VERSION=v0.1.0
```

Tests simulate Atlassian HTTP responses and do not require credentials or mutate
real sites. Release builds produce macOS/Linux (arm64/amd64) and Windows (amd64)
binaries plus SHA-256 checksums under `bin/release/`.

The command registry drives discovery and parsing. `internal/api` handles
authentication, transport, limits, and errors. `internal/cli` separates request
construction, preflight checks, response validation, content conversion, and output.
Shared JSON and file boundaries live in `internal/jsonx` and `internal/fileio`.
See [architecture](docs/architecture.md) and [verification](docs/development.md).

This first version does not include an embedded LLM, MCP server, OAuth, Jira
boards/sprints, bulk operations, self-updates, or Data Center APIs.
See [the short agent guide](docs/agent-guide.md) for efficient usage.

API references: [Jira authentication](https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/),
[enhanced Jira search](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/),
[Confluence pages v2](https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-page/),
[Confluence CQL search](https://developer.atlassian.com/cloud/confluence/rest/v1/api-group-search/),
[Jira attachments](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-attachments/),
[Confluence uploads](https://developer.atlassian.com/cloud/confluence/rest/v1/api-group-content---attachments/),
[Confluence attachment reads](https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-attachment/).
