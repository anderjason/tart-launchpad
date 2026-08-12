# REPO-SECURITY.md

Last reviewed: 2026-08-08

## Scope

This repo is a personal Go CLI/TUI for launching Tart VMs. It shells out to the installed `tart` executable and stores local non-secret preferences.

## Security Model

Tart Launchpad is not a sandbox. It is an intent-preserving wrapper that makes VM launch boundaries visible before execution.

The security-sensitive product behavior is command generation:

- folder access is explicit: `no-folder`, `read-folder`, or `edit-folder`, with a separate resolved project path
- network access is explicit: `offline`, `internet`, `host`, `lan`, or `lan-and-internet`
- clipboard and guest audio are explicit per-run choices and remain off by default
- templates are guarded from normal read-write runs
- temporary runs are cloned from templates and cleaned up afterward

## Dependencies

The app uses Go and Charmbracelet libraries for the TUI. Dependency download, package-manager install, build, and test execution should run in DevWorker unless Jason explicitly approves another environment.

Do not run dependency lifecycle scripts, package-manager installs, or build tooling directly in the normal `codex-agent` account by default.

## Secrets

This repo must not store secrets.

Do not add:

- Tart registry credentials
- GitHub tokens
- SSH keys or agent sockets
- cloud credentials
- package-manager auth files
- personal browser, editor, Codex, or MCP config

Config files may contain VM names, explicit project-folder paths, host-connection choices, LAN CIDRs, and pending temporary cleanup names.

## Tart Boundaries

Do not rely on chmod or private Tart state under `~/.tart` for product behavior.

The current working directory is the default project folder. Users may explicitly choose another folder. Resolve symlinks, reject the filesystem root, the user's home root, and known credential roots, and show the resolved path and access mode during review. This validation does not make Launchpad a sandbox.

Do not add raw `--disk` support or otherwise prepare host storage from Launchpad.

Tart and Softnet own Softnet installation, trust, and privilege requirements. Launchpad passes the reviewed network arguments to Tart and reports Tart's execution error without inspecting or administering the Softnet helper.

## Local Checks

Expected checks:

```bash
go test ./...
go build ./cmd/tart-launchpad
```

For command-planning changes, tests should cover generated Tart command arguments.

## Known Gaps

- `go.mod` pins the module graph, and `go.sum` records dependency checksums. Dependency changes still require review and approved DevWorker checks.
- The first implementation shells out to Tart and does not mock a full VM lifecycle integration test.
- `offline` means outbound IPv4 blocked through Tart Softnet CIDR filtering, not a complete proof of no network activity.
