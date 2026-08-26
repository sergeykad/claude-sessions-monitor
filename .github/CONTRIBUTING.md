# Contributing

Thanks for taking the time to contribute.

## Getting started

```bash
git clone https://github.com/yepzdk/claude-sessions-monitor.git
cd claude-sessions-monitor
make build
```

Go 1.25 or newer is required (`go.mod` pins 1.25.6). The project uses the
standard library plus `golang.org/x/term` — please avoid adding further
external dependencies.

## Workflow

`main` is protected: all changes go through a pull request.

1. Branch off `main` — `feature/short-description`
2. Make your change, and run `make check` — gofmt, `go vet`, `make lint`, the
   build and the tests, which is exactly what CI runs
3. Add an entry to the `[Unreleased]` section of `CHANGELOG.md`
4. Open a pull request

`make lint` runs [golangci-lint](https://golangci-lint.run) against the config
in `.golangci.yml`, once for the host and once for `GOOS=darwin` — the jump
feature is macOS-only, and a Linux-only pass never type-checks it. The pinned
version is built with Go 1.26, so the first run installs that toolchain;
`GOTOOLCHAIN=local` prevents this and the install fails.

The config carries no baseline, so `main` is expected to report zero findings.
A finding that is correct as written gets a `//nolint:<linter>` with the reason
at the site, not an entry in an ignore list.

## Code style

`gofmt` is the only style authority for Go code — it is not configurable and
not a matter of preference, so please don't hand-tune formatting it would
change. Run `gofmt -w .` before committing; CI fails on anything it would
reformat.

This keeps alignment churn (struct tags, map literals) out of feature diffs,
where it otherwise buries the real change. If a diff shows formatting you
didn't intend, it usually means a file was committed unformatted earlier —
fix that in its own commit rather than folding it into a feature change.

An `.editorconfig` covers the non-Go files (JS, CSS, YAML, Markdown); most
editors pick it up automatically.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `perf:`, `ci:`,
`build:`, `style:`.

Example: `fix: resolve stuck Working status on idle sessions`

## Releases

Releases are triggered by pushing a tag, not by merging. Merging to `main`
publishes nothing; a maintainer picks the version from what changed and pushes
`vX.Y.Z`, which builds the binaries and updates the Homebrew formula. You do
not need to bump any version yourself.
