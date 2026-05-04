# Moth

[![CI](https://github.com/alnah/moth/actions/workflows/ci.yml/badge.svg)](https://github.com/alnah/moth/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/alnah/moth)](https://github.com/alnah/moth/releases)
[![Codecov](https://codecov.io/gh/alnah/moth/graph/badge.svg)](https://codecov.io/gh/alnah/moth)
[![Go Reference](https://pkg.go.dev/badge/github.com/alnah/moth.svg)](https://pkg.go.dev/github.com/alnah/moth)

> Moth is an agent-facing command-line interface (CLI) that discovers, fetches,
> extracts, transcribes, and normalizes web and media content as JSON.

Moth runs provider API calls, browser capture, and local acquisition tools from
one JSON-first CLI. Coding agents can use its JSON output. Moth has no cache and no database,
to respect content creators and providers. Teachers, schools, publishers, or learners
should pair it with [Ogmi](https://github.com/alnah/ogmi) to design educational materials
based on authentic documents.

## Status

Moth is v0 software. Commands and JSON structures may change before a stable
release.

Moth is not affiliated with Brave, YouTube, Google, Podcast Index, OpenAI, X,
Chrome, Chromium, yt-dlp, FFmpeg, Poppler, OCRmyPDF, Tesseract, or any content
provider. Provider APIs, websites, and downloaded content keep their own terms
and rights.

## What it does

- Returns JSON by default.
- Searches web, image, and video results through Brave Search.
- Fetches one URL and can extract text, HTML, linked media URLs, screenshots,
  or downloaded assets.
- Searches YouTube, fetches YouTube metadata, downloads subtitles, and extracts
  audio through `yt-dlp`.
- Searches podcasts through Podcast Index, lists episodes, and downloads episode
  audio from RSS/Atom/JSON feeds.
- Searches and fetches X posts, user posts, and username profile lookups.
- Extracts PDF text with optional OCR fallback.
- Transcribes local audio files through OpenAI transcription.
- Starts and controls a persistent Chrome/Chromium browser.
- Captures browser screenshots, PDFs, downloads, response metadata,
  accessibility trees, and manual challenge state.
- Reports external tool availability with `moth tools doctor`.

## Install

### Go install

```sh
go install github.com/alnah/moth/cmd/moth@latest
```

### Release assets

Tagged releases publish archives and checksums on GitHub Releases.

```text
https://github.com/alnah/moth/releases
```

## Quick start

Print the installed version:

```sh
moth version
```

Inspect required and optional external tools:

```sh
moth tools doctor --pretty
```

Fetch and extract one URL:

```sh
moth fetch https://example.com --text --pretty
```

Search the web:

```sh
BRAVE_API_KEY=... moth search web "open data portals" --max-results 5 --pretty
```

Fetch YouTube metadata:

```sh
YOUTUBE_API_KEY=... moth youtube metadata VIDEO_ID --pretty
```

Look up an X user profile by username:

```sh
X_BEARER_TOKEN=... moth x user-lookup alnah --pretty
```

Start a persistent browser and open a page:

```sh
moth browser start --local
moth browser open https://example.com --local --pretty
moth browser pages --local --pretty
moth browser stop --local
```

## Output

Data commands return JSON. The default normalized payload is a `content_pack`:

```json
{
  "type": "content_pack",
  "items": [
    {
      "kind": "page",
      "url": "https://example.com",
      "title": "Example Domain",
      "text": "Example Domain...",
      "warnings": []
    }
  ],
  "warnings": []
}
```

Command-specific fields include `url`, `title`, `text`, `transcript`,
`metadata`, and `warnings`.

Errors are JSON too:

```json
{
  "type": "error",
  "error": {
    "code": "invalid_arguments",
    "message": "unknown flag: --json"
  },
  "warnings": []
}
```

Use `--pretty` for indented JSON and `--output PATH` to write command output to
a file instead of stdout.

## Credentials

Credentials are read from environment variables only. Do not put secrets in the
config file.

| Environment variable | Used by |
| --- | --- |
| `BRAVE_API_KEY` | `moth search ...` |
| `YOUTUBE_API_KEY` | `moth youtube search`, `moth youtube metadata` |
| `PODCASTINDEX_API_KEY` | `moth podcast search`, `moth podcast episodes` |
| `PODCASTINDEX_API_SECRET` | `moth podcast search`, `moth podcast episodes` |
| `X_BEARER_TOKEN` | `moth x ...` |
| `OPENAI_API_KEY` | `moth transcribe` |

## Config

Moth loads a config file only when `--config PATH` is passed. The config file is
JSON and contains non-secret operational settings only.

Example:

```json
{
  "browser": {
    "bin": "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  },
  "limits": {
    "timeout": "30s",
    "max_results": 10,
    "max_bytes": 26214400,
    "retries": 2,
    "retry_base": "500ms",
    "retry_max": "5s"
  }
}
```

Precedence:

1. flags
2. explicit config file
3. environment and defaults

Secrets always come from environment variables.

## External tools

Some commands need local executables. Run `moth tools doctor` to inspect what is
available.

| Tool | Used by |
| --- | --- |
| Chrome/Chromium | browser commands, browser-backed fetch |
| `yt-dlp` | YouTube metadata, subtitles, audio extraction |
| `ffmpeg` | media conversion when needed by acquisition workflows |
| `ffprobe` | media metadata probing |
| `pdftotext` | PDF text extraction |
| `ocrmypdf` | PDF OCR fallback |
| `tesseract` | OCR engine used by `ocrmypdf` |

You can point Moth at a browser with `browser.bin` in the config file or with
`ROD_BROWSER_BIN`.

`ROD_NO_SANDBOX=1` disables the Chromium sandbox. Use it only in trusted CI or
container environments where Chrome cannot start with its sandbox enabled.

## Commands

| Command | Purpose |
| --- | --- |
| `moth search web <query>` | Search web pages. |
| `moth search images <query>` | Search images. |
| `moth search videos <query>` | Search videos. |
| `moth fetch <url>` | Fetch one URL. |
| `moth youtube search <query>` | Search YouTube videos. |
| `moth youtube metadata <video-id>` | Fetch YouTube video metadata. |
| `moth youtube subtitles <url-or-id>` | Download YouTube subtitles. |
| `moth youtube audio <url-or-id>` | Extract YouTube audio. |
| `moth podcast search <query>` | Search Podcast Index. |
| `moth podcast episodes <feed-id>` | List Podcast Index episodes. |
| `moth podcast audio <feed-url> <episode-guid>` | Download one podcast episode enclosure. |
| `moth x search <query>` | Search recent X posts. |
| `moth x post <post-id>` | Fetch one X post. |
| `moth x user <user-id>` | Fetch posts by X user ID. |
| `moth x user-lookup <username>` | Fetch one X profile by username. |
| `moth pdf2txt <file-or-url>` | Extract text from a PDF. |
| `moth transcribe <file>` | Transcribe one local audio file. |
| `moth browser start`, `open`, `pages`, `stop` | Run persistent browser operations. |
| `moth tools doctor` | Inspect external tools. |
| `moth version` | Print the Moth version. |

Run help for details:

```sh
moth --help
moth fetch --help
moth browser --help
```

## Browser state

Persistent browser commands store state under `.moth/browser` for local scope
and `~/.moth/browser` for global scope. State includes the debugger endpoint,
process metadata, and active page identifiers.

Treat browser state as local operational data. Do not share it across machines
or users.

## Security notes

Moth fetches untrusted URLs, starts browsers, executes explicit local tools, and
writes files requested by CLI arguments. Use it locally or in trusted job
runners. Validate inputs before exposing Moth through another service.

Security boundaries:

- credentials are environment-only;
- config files are non-secret;
- downloads and captures are bounded by limits;
- external tools are executed without shell interpolation;
- provider API responses and website content remain untrusted input.

Report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## Development

Use the Makefile for local checks:

```sh
make help
make quick
make check
make ci
```

Common targets:

```sh
make fmt              # format Go files and imports
make lint             # run golangci-lint
make test             # run tests
make test-race        # run race tests
make test-browser     # run browser-tag integration tests
make cover            # write coverage.out and print coverage summary
make cover-browser    # write browser-tag coverage profile
make govulncheck      # check reachable Go vulnerabilities
make goreleaser-check # validate GoReleaser config
make snapshot         # build local snapshot release artifacts
make clean            # remove generated local artifacts
```

Before pushing changes:

```sh
make check
```

Browser-tag CI also runs with `ROD_NO_SANDBOX=1` on GitHub Actions because the
hosted Linux runner cannot use Chrome's sandbox in this setup.

## License

Moth software source code is licensed under the Apache License, Version 2.0. See
[LICENSE](LICENSE).

Third-party services, APIs, websites, media, trademarks, and downloaded content
remain subject to their original terms and rights. See [NOTICE](NOTICE).
