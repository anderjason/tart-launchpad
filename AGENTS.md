# AGENTS.md

## Project

This repo builds Tart Launchpad, a personal terminal UI for running Tart VMs with explicit folder and network boundaries.

## Working Rules

- Read `REPO-SECURITY.md` before running dependency, build, or test commands.
- Keep the product vocabulary consistent across UI, config, tests, and code:
  - VM kinds: `template`, `workspace`, `unmarked`
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
