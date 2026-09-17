---
name: atlo
description: Read, search, and update Jira Cloud issues and Confluence Cloud pages, including comments and attachment uploads, using the atlo CLI.
---

# atlo

Use `atlo` to carry out the requested Jira or Confluence operation. Business
workflows own project, assignee, and status defaults; this skill supplies CLI
mechanics. A Jira reference in an unrelated task doesn't require loading it.

## Discover what you need

Invoke `atlo` from PATH. For an unfamiliar operation, inspect its exact contract
with `atlo schema jira comment update`, for example. Use a namespace such as
`atlo schema confluence` only when the command isn't known. Reuse contracts and
evidence already obtained when still applicable.

Read [setup](references/setup.md) only for executable, authentication, or site
configuration problems. Jira and Confluence may use different sites. If the
installed schema lacks an operation, report the gap rather than inventing flags
or silently changing clients.

## Read economically

Successful output is compact JSON under `data`; failures are JSON on stderr with
a nonzero exit code. Normal output simplifies API records; `--raw` restores the
native response shape when needed.

- Prefer a known key/ID or narrow JQL/CQL query. Start discovery with a small page,
  such as `--limit 5`, and hydrate plausible matches.
- `--fields` limits Jira fields fetched; `--select` limits returned data paths.
  Default issue reads omit descriptions. Request `--fields summary,description`
  when the narrative matters.
- Confluence page reads omit bodies. Use `--body-format storage` when reading or
  editing page content; metadata and search hits aren't the full document.
- Lists return one page. Follow `meta.next_cursor` using `--cursor` and unchanged
  filters as far as the task requires. A non-null cursor means incomplete coverage.
  Required-field discovery needs all relevant metadata pages.

## Write and finish

Read [writing](references/writing.md) when creating or changing content. It covers
the different input formats, attachment uploads, field discovery, and edit-conflict checks.

Use the target and scope authorized by the user. A draft or preview request stays
local; an authorized write doesn't need another confirmation just because this
skill was loaded. `--dry-run` previews a request without network access and checks
local input only. Use it when a preview is useful, not as a required ceremony.

Complete an authorized write, read back the changed fields or content, and report
the key/URL and outcome briefly. If a write fails with an uncertain outcome,
inspect the target before retrying. For an uncertain create, search for the intended
record before creating again. After partial success, continue on the returned ID.
Report unresolved permissions, conflicts, or missing required decisions rather
than repeatedly sending the same failing request.
