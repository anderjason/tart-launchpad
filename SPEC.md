# Tart Launchpad Functional Spec

## Purpose

Tart Launchpad is a personal TUI for running Tart VMs without remembering safety flags. It helps the user choose what a VM may touch, what network it may reach, and whether a VM should be kept or temporary.

It wraps a small, opinionated workflow around `tart list`, `tart clone`, `tart run`, `tart rename`, and `tart delete`.

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
- Templates are guarded from normal runs.
- A template can create a new workspace.
- A template can create a temporary run that is deleted afterward.
- A VM can be renamed or assigned the `template` or `workspace` role.
- Every permanent deletion requires the user to type the VM name.
- A temporary run receives a collision-safe generated name and does not ask the user to name it.
- If `lan` or `lan-and-internet` needs a LAN CIDR, the TUI offers detected private IPv4 network CIDRs first and an `other...` free-text option.
- Config uses the canonical terms above.
- Launchpad stores no secrets.

## Command Mapping

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

## Non-Goals

- No full Tart GUI.
- No registry authentication management.
- No package-manager or guest setup workflows.
- No hidden `safe` profile.
- No mutation of Tart private storage under `~/.tart`.
- No VM resource sizing or display configuration.
