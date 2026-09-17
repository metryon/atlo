# Executable, credentials, and sites

Use the installed `atlo` from PATH. If this skill is linked into an atlo source
checkout, resolve its directory and look for `../../bin/atlo` relative to that
resolved directory. Use that absolute executable path if present. Otherwise
report the missing installation; rebuilding or installing isn't part of ordinary
Jira/Confluence work.

Credentials come from the environment:

| Setting | Shared fallback | Product override |
| --- | --- | --- |
| Site URL | `ATLASSIAN_URL` | `JIRA_URL`, `CONFLUENCE_URL` |
| Email | `ATLASSIAN_EMAIL` | `JIRA_EMAIL` / `JIRA_USER`, `CONFLUENCE_EMAIL` / `CONFLUENCE_USER` |
| API token | `ATLASSIAN_API_TOKEN` | `JIRA_API_TOKEN` / `JIRA_TOKEN`, `CONFLUENCE_API_TOKEN` / `CONFLUENCE_TOKEN` |
| Scoped-token cloud ID | `ATLASSIAN_CLOUD_ID` | `JIRA_CLOUD_ID`, `CONFLUENCE_CLOUD_ID` |

Check the selected product's site against an explicit target URL. Scoped tokens
route through the configured cloud ID; different sites may need different IDs.
`atlo auth check --product jira` or `--product confluence` checks authentication
when needed. Don't dump shell configuration or environment values to diagnose
credentials; report missing variable names without exposing tokens.

Atlo doesn't load `.env` files. Agents inherit their launch process's environment;
shell configuration changes don't update an already-running agent.
