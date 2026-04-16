# code-search-local (`csl`)

Local code search daemon and MCP server. Indexes your local git repositories with [zoekt](https://github.com/sourcegraph/zoekt) and exposes them as:

- a **CLI** for direct `csl search`, `csl count`, `csl repo` use
- an **MCP stdio server** (`csl mcp`) for Claude Code and other MCP clients

All code stays on your machine. No daemon to maintain — a long-running search daemon is spawned on demand and auto-terminates after idle timeout.

## Install

```sh
go install github.com/mad01/code-search-local/cmd/csl@latest
```

Or from a checkout:

```sh
make install   # builds and copies to ~/code/bin/csl
```

## Configure

Create `~/.config/csl/config.yaml`:

```yaml
dirs:
  - ~/code/src/github.com/yourorg
  - ~/workspace
```

## Use

```sh
csl repo --list                  # list discovered repos
csl search "func Walk"           # zoekt query across all repos
csl search "TODO" --repo myrepo  # filter by repo
csl count "fmt.Errorf" --group-by repo
csl doctor                       # check index + daemon health
```

## Register as an MCP server

```sh
claude mcp add --scope user csl -- csl mcp
```

Or use the declarative `sync_mcp.py` reconciler if you have one.

See [`docs/mcp.md`](docs/mcp.md) for the full MCP tool reference.

## License

MIT
