# balpha

Internal BrowserOS alpha dogfooding CLI for running the current checkout against a copied BrowserOS profile.

## What It Does

`balpha` starts a local BrowserOS dogfooding environment:

- Uses the BrowserOS repo path from config, then works from `packages/browseros-agent`.
- Copies one installed BrowserOS profile into a separate dev profile under `~/.config/balpha/profile`.
- Writes `apps/server/.env.production` and `apps/cli/.env.production` from config.
- Runs the existing `tools/dev/setup.sh` setup flow.
- Builds the WXT dev extension.
- Launches `/Applications/BrowserOS.app` with the dev profile, the local extension, and the built-in server disabled.
- Starts the local Bun server from `apps/server`.

It does not auto-pull on `start`. Use `balpha pull` when you want to refresh the checkout.

## Requirements

- macOS.
- Go.
- Bun.
- BrowserOS installed at `/Applications/BrowserOS.app`.
- A BrowserOS monorepo checkout, for example `~/code/browseros-project/browseros-test`.
- `~/bin` or your chosen install directory on `PATH`.

## Install

From the BrowserOS monorepo root:

```bash
cd packages/browseros-agent
bun run install:balpha
```

This builds `tools/alpha/balpha` and installs it to `~/bin/balpha`.

To install somewhere else:

```bash
cd packages/browseros-agent/tools/alpha
make install PREFIX=/usr/local/bin
```

Check the binary:

```bash
balpha --help
```

## First-Time Setup

Run:

```bash
balpha init
```

`init` asks for:

- `Repo path`: the BrowserOS monorepo root, not `packages/browseros-agent`.
- `BrowserOS binary`: defaults to `/Applications/BrowserOS.app/Contents/MacOS/BrowserOS`.
- `Source profile`: selected from the installed BrowserOS profiles in `~/Library/Application Support/BrowserOS`.

Config is written to:

```text
~/.config/balpha/config.yaml
```

The dev profile defaults to:

```text
~/.config/balpha/profile
```

`init` also writes the generated production env files in the configured checkout.

## Start

```bash
balpha start
```

Each start:

- Warns if the configured checkout has uncommitted changes.
- Imports the BrowserOS profile if the dev profile does not exist.
- Rewrites production env files from config.
- Auto-increments busy ports and saves the resolved values back to config.
- Runs `tools/dev/setup.sh`.
- Builds the WXT extension.
- Starts BrowserOS and the local Bun server.

Use this when you want to refresh the copied profile before launching:

```bash
balpha start --refresh-profile
```

Use this for a headless launch:

```bash
balpha start --headless
```

Stop the environment with `Ctrl+C`.

## Update The Checkout

`balpha start` intentionally does not pull. To update the configured repo:

```bash
balpha pull
```

If the checkout has uncommitted changes, `pull` fails. To pull anyway:

```bash
balpha pull --force
```

## Refresh The Copied Profile

To overwrite the dev profile from the selected installed BrowserOS profile:

```bash
balpha refresh-profile
```

This removes and recreates `dev_user_data_dir`. It refuses to run if the dev user-data dir is the real BrowserOS user-data dir or lives inside it.

## Edit Config

```bash
balpha config edit
```

Important fields:

- `repo_path`: BrowserOS monorepo root.
- `browseros_app_path`: BrowserOS executable to launch.
- `source_user_data_dir`: installed BrowserOS user-data dir. Defaults to `~/Library/Application Support/BrowserOS`.
- `source_profile_dir`: installed profile directory to copy.
- `dev_user_data_dir`: separate dev user-data dir. Defaults to `~/.config/balpha/profile`.
- `dev_profile_dir`: dev profile directory. Defaults to `Default`.
- `ports`: CDP, BrowserOS server, and extension ports.
- `production_env`: values written to `apps/server/.env.production` and `apps/cli/.env.production`.

## Safety Notes

- Do not point `dev_user_data_dir` at the real BrowserOS profile.
- `balpha` does not pass `--use-mock-keychain`; copied login data relies on the installed signed app path.
- Default ports are CDP `9015`, server `9115`, and extension `9315`.
- Browser launch passes `--browseros-mcp-port`, `--browseros-server-port`, and `--browseros-proxy-port` to tolerate current switch differences.
