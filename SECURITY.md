# Security Policy

Open Test Sandbox is a local-first workbench that can store requests,
responses, logs, and runtime Evidence. Treat those files as sensitive unless a
profile explicitly proves otherwise.

## Supported Versions

The project has not published a stable release yet. Security fixes are applied
to the main branch until the first public version is tagged.

## Reporting a Vulnerability

Until a public security contact is configured, report vulnerabilities through a
private maintainer channel rather than a public issue. Include:

- affected commit or release;
- reproduction steps;
- whether local Evidence, request data, credentials, or logs are exposed;
- a suggested severity if you have one.

## Handling Local Evidence

- Do not commit `.runtime/`, SQLite files, logs, or generated reports.
- Prefer synthetic examples in public issues and pull requests.
- Redact secrets, tokens, cookies, account numbers, and personal data before
  sharing Evidence outside a trusted environment.
- Keep profile bundles in a separate repository or profile home when they
  contain team-owned endpoints or data.

## Dependency Review

Before a public release, run the release gate and review third-party licenses:

```sh
npm run release-check
go list -m all
npm ls --all
```
