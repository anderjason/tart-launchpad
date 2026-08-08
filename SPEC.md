# Tart Launchpad Functional Spec

## Purpose

Tart Launchpad is a personal TUI for running Tart VMs without remembering safety flags. It helps the user choose what a VM may touch, what network it may reach, and whether a VM should be kept or temporary.

It wraps a small, opinionated workflow around `tart list`, `tart clone`, `tart run`, `tart delete`, `tart export`, and `tart import`.

## Canonical Vocabulary

VM kinds:

- `template`: kept clean; source for new VMs
- `workspace`: kept around; run directly for ongoing work

Folder access:

- `no-folder`: mount no host folder
- `read-folder`: mount the selected project folder read-only
- `edit-folder`: mount the selected project folder read-write
- the project folder initially uses the current directory and may be replaced with another explicit host directory

Host integrations:

- clipboard sharing is an independent per-run choice and is off by default
- guest audio pass-through to the host is an independent per-run choice and is off by default

Volume access:

- mounted host volumes are optional and always unchecked by default
- selected host volumes are shared read-write with Tart `--dir=<name>:/Volumes/<name>`
- Launchpad lists only mounted non-system volumes
- Launchpad does not mount, unmount, chmod, chown, use raw `--disk`, or fall back to another sharing mode

Network access:

- `offline`: outbound IPv4 blocked
- `internet`: Softnet internet access; host/local private networks blocked
- `host`: Tart host-only network
- `lan`: configured LAN CIDRs only; internet blocked
- `lan-and-internet`: internet plus configured LAN CIDRs

Run duration:

- `workspace`: keep VM after run
- `temporary-run`: clone from template, run, then delete afterward

## Required Behavior

- Every run shows the exact `tart` command before execution.
- Workspaces can be run after choosing host connections and network access.
- Folder grants store an explicit resolved project-folder path separately from the access mode.
- Clipboard and guest audio choices are shown with their consequences during review.
- If mounted non-system volumes are connected, the TUI shows a volume checklist after network access and before review.
- Volume choices are all off by default and selected volumes are attached read-write.
- Templates are guarded from normal runs.
- A template can create a new workspace.
- A template can create a temporary run that is deleted afterward.
- A template or workspace can be exported to a user-specified `.tvm` path.
- A `.tvm` file at a user-specified path can be imported with an explicit destination VM name.
- Import and export show the exact Tart command before execution.
- Export refuses to silently overwrite an existing `.tvm` file.
- Import refuses to silently collide with an existing VM name.
- Export review warns that `.tvm` files may contain secrets or sensitive local state.
- If `lan` or `lan-and-internet` needs a LAN CIDR, the TUI offers detected private IPv4 network CIDRs first and an `other...` free-text option.
- Config uses the canonical terms above.
- Launchpad stores no secrets.

## Command Mapping

Before every Launchpad-shaped run, Launchpad configures the VM:

```text
set <vm> --cpu 4 --memory 8192 --display 1280x800
```

Launchpad-shaped runs add these flags when the corresponding connection is off:

```text
--no-clipboard
--no-audio
```

Folder access:

```text
no-folder   no --dir flag
read-folder --dir=project:<selected project folder>:ro
edit-folder --dir=project:<selected project folder>
```

Selected volumes:

```text
--dir=volume-name:/Volumes/Name
```

Network access:

```text
offline          --net-softnet-block=0.0.0.0/0
internet         --net-softnet
host             --net-host
lan              --net-softnet-block=0.0.0.0/0 --net-softnet-allow=<LAN CIDRs>
lan-and-internet --net-softnet --net-softnet-allow=<LAN CIDRs>
```

Template direct run adds:

```text
--root-disk-opts=ro
```

Export:

```text
tart export <vm> <destination>.tvm
```

Import:

```text
tart import <source>.tvm <vm>
```

## Non-Goals

- No full Tart GUI.
- No registry authentication management.
- No package-manager or guest setup workflows.
- No hidden `safe` profile.
- No mutation of Tart private storage under `~/.tart`.
- No custom VM archive format.
- No Tart registry push/pull workflow.
