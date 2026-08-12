# Tart Launchpad Design Language

Tart Launchpad should feel calm, direct, and trustworthy. The interface is a product surface, not a formatted command dump. Each screen should make one decision clear and keep consequences close to that decision.

## Hierarchy

Every screen uses the same order:

1. Neutral product title.
2. Muted location trail, with a right-aligned count only when the count is useful.
3. One cyan question or primary decision.
4. The choices, field, review ledger, or execution state.
5. A short contextual key row.

Use sentence case. Do not add mode labels, ornamental badges, all-caps headings, or prose that repeats the question.

## Palette

Color communicates state or consequence. It never carries meaning alone.

| Token | Terminal colors | Use |
| --- | --- | --- |
| Text | light 235, dark 252 | Product title, primary content |
| Muted | light 240, dark 244 | Trails, section labels, descriptions, inactive state |
| Subtle | light 250, dark 238 | Low-emphasis structure |
| Focus | light 31, dark 86 | Current question, cursor, selected label |
| Attention | light 166, dark 214 | Enabled host reach, warnings, consequences requiring notice |
| Success | light 29, dark 48 | Completed work and running state |
| Danger | light 160, dark 203 | Destructive action labels, failures, delete confirmation |
| Template | light 55, dark 141 | Durable Template identity only |

Prospective choices stay neutral. The current focus is cyan. Amber appears only when access is already enabled or a warning is current. Every colored state also has a label, symbol, or stable position.

## Screen patterns

The product has five screen patterns:

- **Browse:** visible actions first, then Workspaces and Templates.
- **Choose:** one question and a cursor list. Supporting text aligns in a second column on wide terminals and moves below the label on narrow terminals.
- **Text entry:** one question, one field, then either a hint or local validation message.
- **Review:** the decision first, a compact boundary ledger second, and exact commands last in quiet text.
- **Execute and result:** real step, state, and elapsed time. Routine commands are not repeated; a failed command and its output appear with the failure.

The supported minimum terminal is 48 columns by 28 rows. Below 72 columns, choice descriptions stack beneath their labels. The frame is capped at 118 columns so long rows remain readable.

## Interaction

- `↑`/`↓` or `j`/`k` move through lists.
- `enter` opens, chooses, continues, or confirms.
- `esc` goes back one screen.
- `/` filters the VM list.
- `?` opens contextual keys.
- `q` quits from the VM list; `ctrl+c` quits when no operation is active.
- `c` copies commands from Review.

Do not hide settings behind letter shortcuts. A letter accelerator is acceptable only when the same action is visible. Permanent deletion always requires the exact VM name and Enter.

## Product vocabulary

Configuration and CLI values remain stable, but the interface translates them:

| Config value | Interface label |
| --- | --- |
| `no-folder` | None |
| `read-folder` | Read only |
| `edit-folder` | Read and write |
| `offline` | Block outbound IPv4 |
| `internet` | Internet |
| `host` | This Mac |
| `lan` | Local network |
| `lan-and-internet` | Internet + local network |
| `temporary-run` | Temporary |
| `workspace` | Keep as workspace |

Template is a Launchpad role, not a claim that a VM was inspected or cleaned. Describe it as a protected source for creating VMs, and say explicitly that changing a role does not inspect or clean the VM.
