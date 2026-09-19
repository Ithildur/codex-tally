# Codex Tally

[中文](README.md) · English

A local dashboard for Codex token usage, estimated costs, activity, and account quotas. Publish selected statistics to GitHub Pages without keeping your computer online.

- One executable for Windows, Linux, and macOS.
- Separate views for local session usage and account statistics.
- A cost calculator with custom model prices and local usage import.
- Share individual components or a composed hub as HTML, SVG, Markdown, or an iframe.
- English and Chinese interfaces, including shared pages and SVGs.

## Install

Download an archive from [Releases](https://github.com/Ithildur/codex-tally/releases), extract it, and run the executable inside `codex-tally/`.

| Platform | Archive suffix |
| --- | --- |
| Windows | `windows_amd64.zip` / `windows_arm64.zip` |
| Linux | `linux_amd64.tar.gz` / `linux_arm64.tar.gz` |
| macOS, Apple Silicon | `darwin_arm64.tar.gz` |

macOS requires version 13 or later. Windows and Linux builds are available for x86-64 and ARM64. Releases include `SHA256SUMS`. Binaries are currently unsigned.

### Build from source

Requires Go 1.27+. No third-party Go dependencies or frontend build step.

```bash
git clone https://github.com/Ithildur/codex-tally.git
cd codex-tally
go build -trimpath -o codex-tally ./cmd/codex-tally
```

On Windows, use `codex-tally.exe` as the output name. For Pages publishing, fork the project and clone your own repository first.

## Start

Run as the same user who runs Codex:

```bash
./codex-tally
```

On PowerShell, run `.\codex-tally.exe`. Open <http://localhost:4318> and sign in with the password printed in the console after `Login password for this run`. A new 12-character password is generated on every start; no password file is written.

To use a fixed password of at least 8 characters:

```bash
CODEX_TALLY_PASSWORD='your-fixed-password' ./codex-tally
```

```powershell
$env:CODEX_TALLY_PASSWORD = 'your-fixed-password'
.\codex-tally.exe
```

To choose a local IP address and port:

```bash
./codex-tally --host 192.168.1.10 --port 8080
```

Use your machine's actual address. `--host 0.0.0.0` listens on all IPv4 interfaces; `--host ::` listens on IPv6. `HOST` and `PORT` environment variables are also supported; flags take precedence.

By default, the app reads the current user's `.codex` directory. Set `CODEX_HOME` to use another directory. Account statistics require file-based `auth.json`; local statistics work without login credentials.

The interface follows the browser language: Chinese browsers use Chinese, other browsers use English. Use the language selector on the sign-in page, dashboard, or Pages hub to override it. An explicit `?lang=en` or `?lang=zh-CN` takes precedence. Language does not change the reporting timezone or billing rules.

`./codex-tally --version` prints the installed version. The executable requires no Go, Node, or Python runtime. The `sync` command additionally requires Git 2.31+.

## Cost calculator

Open **Calculator**, choose a model or enter custom prices in USD per million tokens. Enter token counts manually, or select a range under **Local** and use **Import local usage**.

Uncached input, cache reads, cache writes, and output are separate categories. Custom calculator prices stay in your browser and do not change the dashboard's pricing configuration. Importing a date range does not treat its cumulative tokens as one long-context request.

## Publish to GitHub Pages

1. Fork and clone your repository, then enable Actions.
2. Select **GitHub Actions** under **Settings → Pages → Build and deployment**.
3. Configure Git identity and authentication. Run this on your repository's default branch:

```bash
./codex-tally sync
```

By default, this publishes the current month's tokens, calls, and cache rate. Model details require an explicit choice. Only `site/usage.json` is committed by `sync`; the Pages URL appears in the Actions deployment.

See the [English guide](docs/guide.en.md) for configuration, offline export, sharing, scheduled sync on all three platforms, pricing, privacy, and releases.

## Contributing

Include the version, operating system, reproduction steps, and sanitized errors in bug reports. Never attach Codex credentials, raw sessions, or private caches. For feature changes, open an issue describing the use case first. Keep pull requests focused and describe behavior changes and validation.

The project uses Go's standard library and embedded HTML/CSS/JavaScript. Implementation lives in `internal/dashboard`; `cmd/codex-tally` is the CLI entry point. See [development and releases](docs/guide.en.md#development-and-releases).

Costs are API token-price estimates, not subscription bills. Account statistics use Codex login endpoints and may change when upstream interfaces change.

## License

[GPL-3.0](LICENSE).
