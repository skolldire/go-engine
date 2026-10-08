# Security Policy

## Supported Versions

`go-engine` is pre-`v1.0` and does not yet offer long-term support branches.
Security fixes are applied to the latest tagged release and to `master`.

| Version | Supported |
|---------|-----------|
| latest `0.x` tag | ✅ |
| older `0.x` tags | ❌ |

## Reporting a Vulnerability

Please **do not** open a public issue for security vulnerabilities.

Report privately through GitHub's
[private vulnerability reporting](https://github.com/skolldire/go-engine/security/advisories/new)
("Report a vulnerability" under the repository's *Security* tab). If that is not
available, contact the maintainer directly.

When reporting, include:

- affected package/path and version or commit,
- a description of the impact,
- steps to reproduce or a proof of concept,
- any known mitigation.

### Response expectations

- Acknowledgement within **5 business days**.
- An initial assessment and severity classification within **10 business days**.
- Coordinated disclosure once a fix is available; credit is given unless you
  prefer to remain anonymous.

## Dependency and vulnerability scanning

- Dependencies are kept up to date via Dependabot (`.github/dependabot.yml`).
- `govulncheck` runs in CI to detect reachable vulnerabilities in dependencies
  and the standard library.
