# Security Policy

## Supported versions

Moth is pre-1.0 software. Users must upgrade to the latest release when a
security fix is published.

Security fixes are not backported to older v0 releases unless a maintainer says
so explicitly in a GitHub Security Advisory or release note.

## Reporting a vulnerability

Do not report security vulnerabilities through public GitHub issues,
discussions, or pull requests.

Use GitHub private vulnerability reporting for this repository. If the GitHub
interface does not expose private reporting, ask for a private contact path in a
public issue without sharing sensitive details.

Include:

- affected version, tag, commit, or release asset;
- operating system and architecture;
- command, flags, config fields, and relevant environment variable names;
- external tool versions from `moth tools doctor`, when relevant;
- steps to reproduce;
- expected and actual behavior;
- impact;
- proof of concept, if available;
- whether the issue affects providers, browser automation, downloads, external
  tools, config loading, output rendering, release artifacts, checksums, or CI
  workflows.

I aim to acknowledge reports within 7 days.

## Scope

Report issues such as:

- credential leakage in logs, JSON output, files, or error messages;
- arbitrary file overwrite, unsafe file permissions, or path traversal;
- unexpected local file read or local file exposure;
- command injection or unsafe external tool invocation;
- unsafe browser launch defaults, browser state exposure, or unintended browser
  profile sharing;
- download, capture, or transcription size-limit bypasses;
- unsafe handling of untrusted provider responses, feed data, PDFs, media files,
  or webpages;
- JSON output issues that can mislead downstream agents about command success,
  errors, provenance, or warnings;
- reachable dependency or Go standard library vulnerabilities;
- release artifact, checksum, tag, or GitHub Actions workflow compromise.

## Out of scope

The following are usually out of scope:

- provider outages, quota limits, billing behavior, or API policy changes;
- provider terms-of-service disputes;
- bugs in third-party services, APIs, browsers, websites, or external tools,
  unless Moth invokes or handles them unsafely;
- malicious URLs, feeds, PDFs, media files, or webpages intentionally supplied by
  a user, unless Moth violates its documented bounds or trust boundaries;
- content quality, legality, or rights disputes for third-party content;
- model or agent output quality when another system consumes Moth JSON;
- vulnerabilities only present in unsupported Go versions or unsupported
  operating systems;
- reports that require already-compromised local credentials or arbitrary local
  code execution outside Moth.

## User responsibilities

Moth is a CLI for local use or trusted job runners. If you expose it through a
server, queue, bot, or agent platform, you are responsible for validating user
input, controlling filesystem paths, isolating browser state, protecting
credentials, and enforcing provider terms and quotas.

Do not put secrets in config files. Moth credentials are environment-only.

Use `ROD_NO_SANDBOX=1` only in trusted CI or container environments where Chrome
cannot start with its sandbox enabled.

## Disclosure

If a vulnerability is confirmed, I will coordinate disclosure with the reporter,
prepare a fix, publish a release, and document the issue in a GitHub Security
Advisory when appropriate.
