# Tart Launchpad

A small personal TUI for running Tart VMs with explicit folder, network, and mounted volume boundaries.

Tart Launchpad asks three practical questions before running a VM:

1. Which VM should run?
2. What folder access should it get?
3. What network access should it get?
4. Should any mounted host volumes be shared read-write?

It then shows the exact `tart` command before execution.

It can also export and import `.tvm` archives at paths you specify. Import/export reviews show the exact Tart command, block silent overwrites or VM-name collisions, and warn that VM archives may contain sensitive state.

## Status

Early-stage tool for local Tart workflows.

## Build

Run builds and dependency downloads in the approved DevWorker environment unless explicitly approved otherwise:

```bash
go test ./...
go build ./cmd/tart-launchpad
```

## Vocabulary

- VM kinds: `template`, `workspace`, `unmarked`
- folder access: `no-folder`, `read-here`, `edit-here`
- network access: `offline`, `internet`, `host`, `lan`, `lan-and-internet`

Mounted volume access is optional, off by default, and uses Tart `--dir=<name>:/Volumes/<name>` directory sharing. Launchpad does not mount, unmount, chmod, chown, use raw `--disk`, or fall back to another sharing mode.

## Softnet Setup

The `offline`, `internet`, `lan`, and `lan-and-internet` modes use Tart Softnet. Softnet may need a one-time admin setup on the host before a non-admin account can use it:

```bash
softnet_path="$(realpath "$(command -v softnet)")"
sudo chown root:wheel "$softnet_path"
sudo chmod 4755 "$softnet_path"
```

Run Launchpad as the normal VM user after that setup. Do not run Launchpad itself as an admin account for ordinary VM work.
