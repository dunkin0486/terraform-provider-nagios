# Security Policy

## Supported Versions

Only the current major version receives security fixes. This provider is
maintained by one person on a best-effort basis - there's no backport
policy for older majors.

| Version | Supported          |
| ------- | ------------------ |
| 2.x     | :white_check_mark: |
| 1.x     | :x:                |

## Reporting a Vulnerability

**Do not open a public GitHub issue for a security vulnerability.** Use
GitHub's private vulnerability reporting instead: go to the
[Security tab](https://github.com/dunkin0486/terraform-provider-nagios/security/advisories/new)
and click "Report a vulnerability". This opens a private draft advisory
that only you and the maintainer can see until it's resolved.

This is a solo-maintained project, so there's no formal SLA - expect an
initial response within a few days, not hours. If the report is accepted,
I'll work with you on a fix and coordinate disclosure timing before
publishing the advisory. If it's declined (not reproducible, out of
scope, etc.), I'll explain why.

In scope: vulnerabilities in this provider's own code (`internal/client`,
`internal/provider`) or its release/build pipeline. Vulnerabilities in
Nagios XI itself are out of scope - report those to
[Nagios Enterprises](https://www.nagios.com/) directly.
