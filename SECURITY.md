# Security policy

## Supported versions

atlo is in early development. Security fixes target the latest published release
and the latest code on `main`; older releases do not receive separate backports.
Update to the latest release, or rebuild from `main`, before checking whether a
problem still occurs.

## Reporting a vulnerability

Use [GitHub's private vulnerability reporting form](https://github.com/metryon/atlo/security/advisories/new).
You can also open the repository's **Security → Advisories → Report a vulnerability**
page. A GitHub account is required. Reports are shared privately with the
repository maintainers through GitHub.

Please include:

- The affected atlo version or commit and operating system.
- The affected command or component and the potential impact.
- Minimal reproduction steps using synthetic credentials and local fixtures.
- Any suggested mitigation, if known.

Do not open a public issue or pull request containing an undisclosed vulnerability.
Do not include real API tokens, private Jira/Confluence content, or confidential
attachments in a report. If a credential has been exposed, revoke it through its
provider; deleting it from a report is not sufficient.

For ordinary bugs and feature requests without security-sensitive details, use
[GitHub Issues](https://github.com/metryon/atlo/issues).

## Security expectations

See [security and operating boundaries](docs/security.md) for the trust model,
protections, and known limitations. Development reviews and automated checks
do not constitute an independent security audit or a guarantee of security.
