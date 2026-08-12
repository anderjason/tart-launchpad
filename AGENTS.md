# AGENTS.md

## Project

This repo builds Tart Launchpad, a personal terminal UI for running Tart VMs with explicit folder and network boundaries.

## Working Rules

- Read `REPO-SECURITY.md` before running dependency, build, or test commands.
- Keep the product vocabulary consistent across UI, config, tests, and code:
  - VM kinds: `template`, `workspace`
  - folder access: `no-folder`, `read-folder`, `edit-folder`
  - network access: `offline`, `internet`, `host`, `lan`, `lan-and-internet`
  - run duration: `workspace`, `temporary-run`
- Do not add secret storage or registry credential handling without a new security review.
- Do not inspect or mutate Tart private storage under `~/.tart` for normal product behavior.
- Prefer small, testable command-planning changes before TUI polish.

## Checks

Run the narrow checks relevant to the change:

```bash
go test ./...
go build ./cmd/tart-launchpad
```

If Go dependencies must be downloaded or package-manager state must change, run those commands in the approved DevWorker environment.

## Release Build

Build releases with Tart DevWorker using this process:

```bash
DEVWORKER_PROVIDER=tart devworker worker run "$PWD" -- '
  set -e
  go test ./...
  mkdir -p "$DEVWORKER_ARTIFACTS/release"
  go build -trimpath -ldflags="-s -w" \
    -o "$DEVWORKER_ARTIFACTS/release/tart-launchpad" \
    ./cmd/tart-launchpad
  "$DEVWORKER_ARTIFACTS/release/tart-launchpad" --help >/dev/null
  tar -C "$DEVWORKER_ARTIFACTS/release" \
    -czf "$DEVWORKER_ARTIFACTS/tart-launchpad-darwin-arm64.tar.gz" \
    tart-launchpad
  shasum -a 256 \
    "$DEVWORKER_ARTIFACTS/release/tart-launchpad" \
    "$DEVWORKER_ARTIFACTS/tart-launchpad-darwin-arm64.tar.gz"
'
```

After DevWorker pulls the artifacts, copy the binary and archive from that run's `explicit/` artifact directory to:

```text
dist/tart-launchpad
dist/tart-launchpad-darwin-arm64.tar.gz
```

Verify the copied files with `shasum -a 256`. Use this process as written instead of composing a different release command for each build.
