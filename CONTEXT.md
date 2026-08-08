# Tart Launchpad Context

Tart Launchpad is a personal terminal interface for launching Tart virtual machines with explicit host access, network access, and lifetime choices.

## Language

**Launchpad-shaped run**:
A Tart VM run that shows explicit access choices and disables clipboard and audio by default.
_Avoid_: safe run, sandboxed run

**VM kind**:
The Launchpad label for how a local Tart VM should be treated.
_Avoid_: category, type

**Template**:
A VM kind for a clean VM used as the source for new VMs or read-only runs.
_Avoid_: base image

**Workspace**:
A VM kind for a VM that is kept around and run directly for ongoing work.
_Avoid_: project VM

**Host access grant**:
The combined project-folder, mounted-volume, clipboard, and guest-audio connections given to a Launchpad-shaped run.
_Avoid_: mount policy, storage profile

**Folder access**:
The read/write choice for the selected project folder in a Launchpad-shaped run.
_Avoid_: directory mode

**Project folder**:
The one explicit host directory shared with a Launchpad-shaped run. It initially uses the launch directory but can be replaced.
_Avoid_: here, current directory

**Clipboard sharing**:
The per-run choice that lets clipboard contents cross between the host and guest.
_Avoid_: clipboard access

**Guest audio**:
The per-run choice that lets guest audio play through the host.
_Avoid_: microphone access

**Volume access**:
The optional mounted-host-volume access chosen for a Launchpad-shaped run.
_Avoid_: disk access

**Network access**:
The network reachability choice for a Launchpad-shaped run.
_Avoid_: network profile

**Run duration**:
Whether a VM is kept as a workspace or deleted after a temporary run.
_Avoid_: lifecycle

**VM archive**:
A `.tvm` file exported from or imported into Tart.
_Avoid_: backup, image bundle

**Launchpad flow**:
The sequence of product choices that turns a selected VM or VM archive into a reviewable plan.
_Avoid_: wizard

**Reviewable plan**:
The generated Tart commands plus the product facts needed to review access, warnings, prerequisites, and cleanup behavior before execution.
_Avoid_: command list

**Host environment**:
The host facts Launchpad reads, such as the current project path, mounted volumes, and detected LAN CIDRs.
_Avoid_: system service

## Relationships

- A **Launchpad-shaped run** has exactly one **Host access grant**.
- A **Launchpad-shaped run** has exactly one **Network access** choice.
- A **Template** can create a **Workspace** or a temporary **Launchpad-shaped run**.
- A **Reviewable plan** is produced by the **Launchpad flow** before command execution.
- A **Host environment** supplies local facts to the **Launchpad flow**.
- A **VM archive** can be exported from a **Template** or **Workspace**.

## Example Dialogue

> **Dev:** "When the user chooses a **Template**, can the **Launchpad flow** run it directly?"
> **Domain expert:** "Only as a read-only **Launchpad-shaped run**. A normal run should create a **Workspace** or a temporary run."

> **Dev:** "Should selected USB drives be part of **Folder access**?"
> **Domain expert:** "No. They are **Volume access**, but both choices combine into one **Host access grant** for review and command generation."

## Flagged Ambiguities

- "safe" is not a product term. Launchpad makes access visible, but it is not a sandbox.
- "backup" is not used for exported `.tvm` files. Use **VM archive** because archives may contain sensitive local state.
