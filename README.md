# Tart Launchpad

Choose what a Tart VM can touch before you run it.

Tart Launchpad is a terminal interface for local Tart VMs. It asks about host-folder access, network access, and optional mounted volumes. Before it runs, imports, or exports anything, it shows the exact Tart command.

## Requirements

- macOS with [Tart](https://tart.run/) installed and available in `PATH`.
- Go 1.22 or newer to build from source.
- Tart Softnet only when you choose `offline`, `internet`, `lan`, or `lan-and-internet` network access.

## Build and run

```sh
go build -o tart-launchpad ./cmd/tart-launchpad
./tart-launchpad
```

## Use it

1. Select a VM. Launchpad treats ordinary VMs as workspaces by default. Mark a clean source VM as a template when you want to create new VMs from it.
2. Choose folder access, network access, and any mounted volumes to share.
3. Review the generated commands, then run them.

Templates can create a workspace or a temporary VM, or run with a read-only root disk. Import and export each ask for a `.tvm` path before review.

## Plan without running

Use `plan` when you only want to see the commands:

```sh
./tart-launchpad plan \
  --vm example-vm \
  --folder-access read-here \
  --network-access offline
```

The command prints the Tart commands and does not execute them.

## Access choices

Folder access applies to the directory where you start Launchpad:

- `no-folder`: share no host folder.
- `read-here`: share the current directory read-only.
- `edit-here`: share the current directory read-write.

Network access:

- `offline`: block outbound IPv4 through Softnet.
- `internet`: allow internet access through Softnet; block the host and local private networks.
- `host`: use Tart's host-only network.
- `lan`: allow configured local network ranges only.
- `lan-and-internet`: allow configured local network ranges and the internet.

Mounted host volumes are always off by default. Selected volumes are shared read-write. Launchpad never mounts, unmounts, or prepares host storage, and it does not use raw disks.

Launchpad turns clipboard and audio off for every run. It is not a security sandbox.

## Softnet setup

Launchpad checks Softnet before an operation that needs it. If setup is required, run this once from an administrator shell:

```sh
softnet_path="$(realpath "$(command -v softnet)")"
sudo chown root:wheel "$softnet_path"
sudo chmod 4755 "$softnet_path"
```

Then run Launchpad as your normal user.
