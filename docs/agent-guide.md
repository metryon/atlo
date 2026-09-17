# Using atlo with agents

The reusable [atlo skill](../skills/atlo/SKILL.md) is the maintained agent guide.
Its entrypoint covers discovery, economical reads, and completion; supporting
references cover writing and setup only when those details are needed.

Link or copy `skills/atlo` into your agent's skill directory. For this development
checkout, Pi and Codex can share the same source through directory symlinks.
Keep business-specific routing and defaults in separate workflow skills.

The design follows [Rethinking skills and prompts for GPT-6 Astra](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra):
a focused description, conditional references, and decision boundaries rather
than a fixed itinerary. Exact commands and flags remain discoverable through
`atlo schema <command>`.

Use `meta.next_cursor` to decide whether to fetch another page; a short page can
still have a continuation. Malformed pagination produces `invalid_response`, so
do not describe an errored search as complete. A preflight error explicitly saying
that no write was sent means the guard rejected the operation. Other write errors
may have an uncertain outcome; inspect the target before retrying. See the
[security documentation](security.md) for the protections and remaining limits.
