# Contributing

Thank you for helping improve Tart Launchpad.

## Before you begin

Use a public issue for a bug report, a focused improvement, or a feature idea.
For a new feature, a public behavior change, or a substantial refactor, discuss
the approach in an issue before writing code.

Do not report security issues in a public issue. Follow [the security policy](SECURITY.md).

## Pull requests

Keep each pull request focused on one change. Explain what changed, why it
matters, and how you tested it. Include a screenshot or recording when a change
affects the terminal interface.

Run these checks when your environment supports them:

```sh
go test ./...
go build ./cmd/tart-launchpad
```

## AI-assisted work

AI-assisted contributions are welcome. The contributor remains responsible for
the pull request: understand the change, run the relevant checks, and be able
to explain the behavior and tradeoffs during review.

Do not submit a generated change that you cannot explain or validate.

## Security-sensitive changes

Call out changes to command planning, host-folder access, network
access, GitHub configuration, dependencies, build scripts, or release behavior.
These changes receive closer review because they can affect a user's machine or
the project's supply chain.
