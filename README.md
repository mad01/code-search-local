# code-search-local (`csl`)

> Local code search over your git checkouts. A CLI and an MCP stdio server for Claude Code, backed by [zoekt](https://github.com/sourcegraph/zoekt).

`csl` walks the directories you configure, indexes every git repo it finds, and searches them with zoekt's trigram index. You get `grep`-like queries across dozens of checkouts in milliseconds, without shipping your code to a cloud service.

There is no server to manage. A short-lived search daemon is spawned on demand from the same binary, keeps zoekt shards mmap'd across queries, and exits after ten minutes of idleness.

Use it in two ways:

- At the terminal: `csl search`, `csl count`, `csl repo`, `csl read`.
- Inside Claude Code (or any MCP client). `csl mcp` exposes eight `csl_*` tools so the agent can search, resolve repo paths, and read files without shelling out.

## Install

Requires Go 1.25+.

```sh
go install github.com/mad01/code-search-local/cmd/csl@latest
```

From a checkout:

```sh
make install   # builds with version embedded, copies to ~/code/bin/csl
```

## Configure

Create `~/.config/csl/config.yaml` listing the directories that contain your git checkouts:

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace
```

`csl` walks each directory concurrently and records every directory whose immediate child is `.git`. Nested repos are not descended into once a `.git` is found.

## Use the CLI

```sh
csl repo --list                     # list every indexed repo
csl search "func Walk"              # search all indexed repos
csl search "TODO" --repo myrepo     # filter to one repo
csl search "fmt\.Errorf" --lang go --output-mode content -C 3
csl count "TODO" --group-by repo    # cross-repo tally
csl doctor                          # check index + daemon health
```

First search in a fresh checkout triggers an initial index build. Subsequent searches are served by the daemon and return in hundreds of milliseconds.

## Register the MCP server

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list   # expect: csl: csl mcp - ✓ Connected
```

The MCP server exposes eight tools: search, count, query validation, repo lookup/info/pull/reindex, and file read. See [docs/mcp.md](docs/mcp.md) for the per-tool reference.

## Docs

- [Getting started](docs/getting-started.md) — install, configure, first search.
- [Configuration](docs/configuration.md) — config file, paths, environment.
- [CLI reference](docs/cli.md) — every subcommand and flag.
- [MCP server reference](docs/mcp.md) — `csl mcp` tool reference for Claude Code.
- [Architecture](docs/architecture.md) — how the daemon, index, and MCP adapter fit together.

## License

MIT. See [LICENSE](LICENSE).
