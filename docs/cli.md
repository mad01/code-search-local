# CLI reference

Every `csl` subcommand, grouped by purpose. Flags use long form unless a short flag exists. Required flags are marked in the table.

Every command that reads repos loads `~/.config/csl/config.yaml` and walks the configured `dirs`. See [configuration](configuration.md) for the schema.

## Index summary

| Command | Purpose |
|---|---|
| [`csl search`](#csl-search) | Search code across all indexed repos |
| [`csl count`](#csl-count) | Count matches, optionally grouped by repo or language |
| [`csl read`](#csl-read) | Read a file from a named repo by relative path |
| [`csl repo`](#csl-repo) | List or interactively pick a repo |
| [`csl index`](#csl-index) | Manage the zoekt index (status, repair, clean) |
| [`csl doctor`](#csl-doctor) | Report index and daemon health |
| [`csl query`](#csl-query) | Validate a zoekt query without running it |
| [`csl mcp`](#csl-mcp) | Start the MCP stdio server (for Claude Code) |
| [`csl version`](#csl-version) | Print the build version |

---

## `csl search`

Search code across all indexed repos using zoekt query syntax.

### Synopsis

```sh
csl search [flags] <pattern>
csl search --serve                 # start daemon in foreground
csl search --stop                  # stop the running daemon
```

### Description

`csl search` tries the daemon first, falling back to an in-process search if the socket is unreachable. On the first search after a clean install (or after `csl index --clean`), `csl` indexes every configured repo synchronously and reports progress on stderr, then runs the query.

After every search, stale repos (new commits, dirty tree, branch switched) are re-indexed in the background before the next call.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-r, --repo <regex>` | `""` | Restrict to repos matching this regex or substring |
| `-l, --lang <name>` | `""` | Restrict to a single language (e.g. `go`, `python`, `swift`) |
| `-f, --file <regex>` | `""` | Restrict to file paths matching this regex (e.g. `\.go$`) |
| `-o, --output-mode <mode>` | `files_with_matches` | `files_with_matches` prints unique paths; `content` prints matching lines |
| `-C, --context-lines <n>` | `0` | Context lines around each match. Only applies to `content` mode |
| `--limit <n>` | `50` | Maximum number of file results |
| `--case-sensitive` | `false` | Force case-sensitive matching. Default is smart case (case-insensitive unless the query has uppercase) |
| `--reindex` | `false` | Synchronously re-index before searching |
| `--json` | `false` | Emit results as a JSON array of match objects |
| `--toon` | `false` | Emit results as [TOON](https://github.com/alpkeskin/gotoon), a compact format designed for LLM consumption |
| `--serve` | `false` | Run the search daemon in the foreground (blocks until `SIGINT` or idle timeout) |
| `--stop` | `false` | Send a shutdown request to the running daemon |

### Query syntax

`csl` passes the pattern to zoekt's query parser. The common operators:

| Syntax | Meaning |
|---|---|
| `foo` | Literal substring match |
| `foo.*bar` | Regular expression |
| `"func Walk"` | Quoted phrase — exact match with spaces |
| `foo bar` | Space-separated terms are AND |
| `foo\|bar` | OR |
| `-test` | NOT |
| `repo:<regex>` | Restrict to repos matching |
| `file:\.go$` | Restrict to file paths matching |
| `lang:go` | Restrict to a language |
| `case:yes` | Case-sensitive this term |

Regex metacharacters inside the pattern need shell-escaping too. Validate a tricky query first with [`csl query`](#csl-query).

### Examples

**Find a function across every repo:**

```sh
csl search "func Walk"
```

**Narrow to one repo and one language, show surrounding context:**

```sh
csl search "fmt\.Errorf" --repo code-search-local --lang go --output-mode content -C 3
```

**Machine-readable output for piping into tools:**

```sh
csl search "import.*cobra" --file "\.go$" --limit 10 --json
```

**Force a re-index before searching, useful after a manual git checkout:**

```sh
csl search --reindex "NewBuilder"
```

---

## `csl count`

Count matches across indexed repos, optionally grouped.

### Synopsis

```sh
csl count [flags] <pattern>
```

### Description

Takes the same query syntax as `csl search`. Always goes through the daemon first, falling back to in-process. Exits with a single total unless `--group-by` is set.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-r, --repo <regex>` | `""` | Restrict to repos matching |
| `-l, --lang <name>` | `""` | Restrict to a language |
| `--group-by <key>` | `""` | Group counts by `repo` or `language` |
| `--json` | `false` | Emit results as JSON |

### Examples

**Single total:**

```sh
csl count "TODO"
```

**Per-repo breakdown:**

```sh
csl count "func.*Error" --group-by repo
```

Output:

```
mad01/code-search-local                  87
myorg/service-a                          55

total: 142
```

**Per-language breakdown, filtered to Go files:**

```sh
csl count "import" --lang go --group-by language
```

---

## `csl read`

Read a file from a named local repo.

### Synopsis

```sh
csl read --repo <name> [flags] <path>
```

### Description

Resolves `--repo` against every configured repo using substring match (not regex). The file path is relative to the repo root. Lines are 1-based; `--start-line 0` or `--end-line 0` means open-ended.

### Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `-r, --repo <name>` | yes | — | Repo name to read from. Substring match |
| `--start-line <n>` | no | `0` | First line to show (1-based, 0 for start) |
| `--end-line <n>` | no | `0` | Last line to show (1-based inclusive, 0 for end) |
| `--json` | no | `false` | Emit as JSON: `{repo, path, lines: [{number, text}]}` |

### Examples

```sh
csl read internal/cli/root.go --repo code-search-local
csl read main.go --repo code-search-local --start-line 10 --end-line 30
csl read README.md --repo code-search-local --json
```

---

## `csl repo`

Discover and pick git repos.

### Synopsis

```sh
csl repo                    # interactive fuzzy finder
csl repo --list             # tab-separated name + path
csl repo --json             # structured output
csl repo --toon             # TOON-encoded output for LLMs
```

### Description

Without flags, `csl repo` opens a [fuzzy finder](https://github.com/ktr0731/go-fuzzyfinder) and prints the absolute path of the selected repo on stdout, useful for `cd $(csl repo)` workflows.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--list` | `false` | Print every repo, tab-separated: `name<TAB>path` |
| `--json` | `false` | Emit array of `{name, path, remote, host}` (implies `--list`) |
| `--toon` | `false` | TOON-encoded output under a `repos` key (implies `--list`) |

### Examples

```sh
cd $(csl repo)                      # pick and cd
csl repo --list | grep service-
csl repo --json | jq '.[] | .host' | sort -u
```

---

## `csl index`

Manage the zoekt index.

### Synopsis

```sh
csl index                   # re-index stale repos only
csl index --all             # force re-index of every repo
csl index --status          # report per-repo freshness
csl index --repair          # validate and drop corrupted shards
csl index --clean           # delete the entire index directory
```

### Description

By default, `csl index` diffs the current repo fingerprints against `state.json` and re-indexes only the repos whose fingerprint changed. The fingerprint covers HEAD, branch, and `git status --porcelain`, so any commit, checkout, or working-tree change triggers a re-index.

`--repair` opens every `*.zoekt` shard, parses its metadata, and removes any that fail to read. Run `csl index` after repair to rebuild affected repos.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--all` | `false` | Re-index every repo, regardless of fingerprint |
| `--status` | `false` | Print a status table instead of indexing |
| `--clean` | `false` | Remove `~/.config/csl/search-index/` entirely |
| `--repair` | `false` | Validate shards, remove corrupted ones |
| `--json` | `false` | JSON output for `--status` and `--repair` |

### Examples

**Status table:**

```sh
csl index --status
```

Output:

```
REPO                                     STATUS     DIRTY    INDEXED AT           BRANCH
mad01/code-search-local                  fresh      no       2m12s ago            main
myorg/service-a                          stale      yes      1h4m ago             feat/auth
```

**Full rebuild:**

```sh
csl index --all
```

**Repair after a crash or bad disk:**

```sh
csl index --repair
csl index
```

---

## `csl doctor`

Report index and daemon health in one view.

### Synopsis

```sh
csl doctor [--json]
```

### Description

`doctor` combines everything `csl index --status` reports plus shard integrity, daemon state, and total index size. It classifies issues into `stale`, `missing`, and `dirty`. Run it when searches look wrong or slow.

### Example output

```
Index directory: /Users/you/.config/csl/search-index
Index size:      84.3 MB
Total repos:     17
Index shards:    17 (17 healthy, 0 corrupted)
Search daemon:   running

Issues (1):
  dirty    myorg/service-a                          3 modified, 1 untracked

Healthy (16):
  mad01/code-search-local                  indexed 2m12s ago
  ...
```

If corrupted shards are reported, the output suggests `csl index --repair` followed by `csl index`.

---

## `csl query`

Validate and parse a zoekt query without running it.

### Synopsis

```sh
csl query [--json] <pattern>
```

### Description

Useful when the query has escaped regex metacharacters and you want to confirm the parser sees what you mean. Returns the parsed tree on success, or the parse error plus a fixing hint on failure.

### Examples

```sh
csl query "func Walk"
# valid: true
# parsed: (and substr:"func Walk")

csl query "func \(Walk"
# valid: false
# error: ...
# hint: Escape special regex characters with backslash, e.g. "func \(Walk"
```

---

## `csl mcp`

Start the MCP stdio server.

### Synopsis

```sh
csl mcp
```

### Description

Reads JSON-RPC 2.0 requests from stdin, writes responses to stdout, and exits when stdin closes. The MCP client spawns one per session; there is no persistent server.

Register with Claude Code:

```sh
claude mcp add --scope user csl -- csl mcp
```

Smoke test without an MCP client:

```sh
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | csl mcp
```

Expected: an `initialize` response followed by `tools/list` listing eight `csl_*` tools with their input/output schemas. See [MCP reference](mcp.md) for per-tool details.

---

## `csl version`

Print the version string.

### Synopsis

```sh
csl version
```

The version is set at build time via `-ldflags "-X github.com/mad01/code-search-local/internal/cli.Version=<value>"`. `make install` sets it to the short git hash; `go install` without ldflags yields `dev`.
