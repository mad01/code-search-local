# MCP reference

`csl mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) stdio server bundled into the `csl` binary. It exposes the same search, repo, and file-read functionality as the cobra subcommands, but as typed MCP tools that Claude Code (and other MCP clients) can call directly.

There is no persistent server. The MCP client spawns `csl mcp` as a subprocess per session, pipes JSON-RPC 2.0 over stdin/stdout, and the process exits when stdin closes.

## Register

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list
```

Expected:

```
csl: csl mcp - ✓ Connected
```

The registration stores the command name (not an absolute path), so `csl` must be on the `PATH` of whatever process launches Claude Code.

## Tools

Eight tools are registered, grouped into three areas: `csl_repo_*` for repo management, `csl_search` / `csl_count` / `csl_query_validate` for search, and `csl_read` for file reads.

| Tool | Purpose |
|---|---|
| [`csl_repo_lookup`](#csl_repo_lookup) | Resolve a repo name to its local checkout path |
| [`csl_repo_info`](#csl_repo_info) | Git and index health for a repo, plus a suggested action |
| [`csl_repo_pull`](#csl_repo_pull) | `git pull --ff-only` with safety checks |
| [`csl_repo_reindex`](#csl_repo_reindex) | Rebuild the zoekt index for a single repo |
| [`csl_search`](#csl_search) | Search code across locally checked-out repos |
| [`csl_count`](#csl_count) | Count matches, optionally grouped by repo or language |
| [`csl_read`](#csl_read) | Read a file from a named local repo |
| [`csl_query_validate`](#csl_query_validate) | Validate a zoekt query and return its parsed tree |

### `csl_repo_lookup`

Resolve a repo name to its absolute local checkout path.

**When to call:** the user mentions a repo by name and you need its path before `cd`-ing, reading, or grepping inside it.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Case-insensitive regex or substring matched against `org/repo` (e.g. `tboi`, `mad01/.*`) |

**Output:**

| Field | Type | Description |
|---|---|---|
| `matches` | array | Matching repos; empty if no local checkout was found |
| `matches[].name` | string | `org/repo` name extracted from the git remote URL |
| `matches[].path` | string | Absolute filesystem path to the repo root |
| `matches[].remote` | string | Full origin remote URL (e.g. `git@github.com:mad01/csl.git`) |
| `matches[].host` | string | Hostname extracted from the remote URL (e.g. `github.com`) |

**Example:**

```json
// request
{"name": "code-search-local"}

// response
{
  "matches": [{
    "name": "mad01/code-search-local",
    "path": "/Users/alex/code/src/github.com/mad01/code-search-local",
    "remote": "git@github.com:mad01/code-search-local.git",
    "host": "github.com"
  }]
}
```

An empty `matches` means the repo is not checked out under any configured `dirs`. Report that to the user rather than guessing a path.

### `csl_repo_info`

Report git state and index state for a repo, plus a suggested next action.

**When to call:** before creating a branch, running `git pull`, or making edits, to confirm the working tree is clean and the index is fresh.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Case-insensitive regex or substring matched against the repo name |

**Output:**

| Field | Type | Description |
|---|---|---|
| `matches` | array | Matching repos, each with git and index state |
| `matches[].name` | string | `org/repo` name |
| `matches[].path` | string | Absolute filesystem path |
| `matches[].remote` | string | Origin remote URL |
| `matches[].host` | string | Git host |
| `matches[].branch` | string | Current git branch (or `HEAD` if detached) |
| `matches[].dirty` | bool | Working tree has uncommitted changes |
| `matches[].modified_files` | int | Count of modified tracked files |
| `matches[].untracked_files` | int | Count of untracked files |
| `matches[].index_stale` | bool | Zoekt index is behind the current fingerprint |
| `matches[].indexed_at` | string | RFC3339 timestamp, or the literal string `never` |
| `matches[].action` | string | One of `ready`, `commit_or_stash`, `pull_recommended`, `needs_reindex` |

The `action` field encodes the decision tree:

- `commit_or_stash` — the working tree is dirty.
- `needs_reindex` — never indexed, or index fingerprint does not match current.
- `pull_recommended` — indexed more than 30 minutes ago; likely behind remote.
- `ready` — everything else.

**Example:**

```json
{
  "matches": [{
    "name": "mad01/code-search-local",
    "path": "/Users/alex/code/src/github.com/mad01/code-search-local",
    "branch": "main",
    "dirty": false,
    "modified_files": 0,
    "untracked_files": 0,
    "index_stale": false,
    "indexed_at": "2026-04-16T12:03:44Z",
    "action": "ready"
  }]
}
```

### `csl_repo_pull`

Run `git pull --ff-only` in a named repo, with safety checks for dirty trees and detached HEAD.

**When to call:** before creating a branch on a repo that may be behind.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Case-insensitive substring match against the repo name |
| `force` | bool | no | Pull even if the working tree is dirty or HEAD is detached |

**Output:**

| Field | Type | Description |
|---|---|---|
| `name` | string | `org/repo` name |
| `path` | string | Absolute filesystem path |
| `branch` | string | Current branch |
| `updated` | bool | `true` if HEAD moved after the pull |
| `warning` | string | Set when a safety check blocked the pull |
| `old_head` | string | HEAD before the pull |
| `new_head` | string | HEAD after the pull |

**Successful fast-forward:**

```json
{
  "name": "mad01/code-search-local",
  "path": "/Users/alex/code/src/github.com/mad01/code-search-local",
  "branch": "main",
  "updated": true,
  "old_head": "a1b2c3d",
  "new_head": "e4f5g6h"
}
```

**Blocked by dirty tree:**

```json
{
  "name": "mad01/code-search-local",
  "path": "/Users/alex/code/src/github.com/mad01/code-search-local",
  "branch": "main",
  "updated": false,
  "warning": "uncommitted changes (2 modified, 1 untracked) — use force=true to pull anyway"
}
```

The pull is always `--ff-only`. If a real merge would be needed, the underlying `git` command fails and the error surfaces to the client.

### `csl_repo_reindex`

Rebuild the zoekt index for a single repo.

**When to call:** after a significant edit or checkout, when `csl_repo_info` reports `action: needs_reindex`, or when fresh results are needed sooner than the background re-indexer would catch up.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Case-insensitive substring match against the repo name |

**Output:**

| Field | Type | Description |
|---|---|---|
| `name` | string | `org/repo` name |
| `path` | string | Absolute filesystem path |
| `reindexed` | bool | `true` if indexing succeeded |
| `duration` | string | Human-readable indexing duration (e.g. `1.234s`) |

**Example:**

```json
{
  "name": "mad01/code-search-local",
  "path": "/Users/alex/code/src/github.com/mad01/code-search-local",
  "reindexed": true,
  "duration": "1.234s"
}
```

Only the named repo's shards are touched. Other repos in the index are untouched. State is updated with the new fingerprint and timestamp.

### `csl_search`

Search code across locally checked-out repos using zoekt query syntax.

**When to call:** "where is X defined", "find all Y", "does any of my projects use Z", "show me every TODO in the Go code".

**Input:**

| Field | Type | Default | Description |
|---|---|---|---|
| `query` | string | — | Zoekt query |
| `repo` | string | `""` | Restrict to repo names matching this regex |
| `lang` | string | `""` | Restrict to files of this language |
| `file` | string | `""` | Restrict to file paths matching this regex |
| `output_mode` | string | `files_with_matches` | `files_with_matches` or `content` |
| `context_lines` | int | `0` | Context lines per match. Only applies to `content` mode |
| `limit` | int | `50` | Maximum number of file results |
| `case_sensitive` | bool | `false` | Force case-sensitive matching; default is smart case |

**Output (`files_with_matches`):**

```json
{
  "output_mode": "files_with_matches",
  "files": [
    {"repo": "mad01/code-search-local", "path": "internal/mcpserver/server.go"}
  ],
  "total": 1
}
```

**Output (`content`):**

```json
{
  "output_mode": "content",
  "lines": [
    {
      "repo": "mad01/code-search-local",
      "path": "internal/mcpserver/server.go",
      "line": 12,
      "column": 6,
      "text": "func New(version string) *mcp.Server {",
      "before": "...",
      "after": "..."
    }
  ],
  "total": 1
}
```

**Query syntax:**

| Syntax | Meaning |
|---|---|
| `foo` | Literal substring |
| `"foo bar"` | Quoted phrase |
| `foo bar` | AND (space-separated) |
| `foo\|bar` | OR |
| `-foo` | NOT |
| `file:\.go$` | File path regex |
| `lang:go` | Language filter |
| `repo:kitty` | Repo name regex |
| `case:yes` | Case-sensitive match |

Use [`csl_query_validate`](#csl_query_validate) to debug complex queries.

### `csl_count`

Count matches across locally checked-out repos, optionally grouped.

**When to call:** cross-repo tallies like "how many TODOs across my Go projects" or "which language has the most calls to `fmt.Errorf`".

**Input:**

| Field | Type | Description |
|---|---|---|
| `query` | string | Zoekt query (same syntax as `csl_search`) |
| `repo` | string | Restrict to repos matching this regex |
| `lang` | string | Restrict to files of this language |
| `group_by` | string | `repo`, `language`, or empty for a single total |

**Output:**

```json
{
  "total": 142,
  "groups": [
    {"group": "mad01/code-search-local", "count": 87},
    {"group": "myorg/service-a", "count": 55}
  ]
}
```

`groups` is empty when `group_by` is not set.

### `csl_read`

Read a file from a named local repo by repo name and relative path.

**When to call:** you already know which repo holds a file and want a specific line range without first resolving the absolute path via `csl_repo_lookup`.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `repo` | string | yes | Substring match against the `org/repo` name |
| `file` | string | yes | File path relative to the repo root |
| `start_line` | int | no | First line to return, 1-based; `0` or omit for start of file |
| `end_line` | int | no | Last line to return, 1-based inclusive; `0` or omit for end of file |

**Output:**

```json
{
  "repo": "mad01/code-search-local",
  "path": "internal/mcpserver/server.go",
  "lines": [
    {"number": 12, "text": "func New(version string) *mcp.Server {"}
  ]
}
```

The line scanner buffers up to 1 MB per line, so files with very long minified lines still read cleanly.

### `csl_query_validate`

Validate a zoekt query and return its parsed tree, or a parse error with a fixing hint.

**When to call:** to debug a complex query before invoking `csl_search`, especially when regex metacharacters are involved.

**Input:**

| Field | Type | Description |
|---|---|---|
| `query` | string | The zoekt query string to validate |

**Output (valid):**

```json
{
  "valid": true,
  "parsed": "(and substr:\"foo\" file_regexp:\"\\.go$\")"
}
```

**Output (invalid):**

```json
{
  "valid": false,
  "error": "unexpected end of query",
  "hint": "check for unbalanced quotes or parentheses"
}
```

## Troubleshooting

**`claude mcp list` shows `csl: csl mcp - ✗ Failed to connect`**
Run `which csl` in the same shell that launched Claude Code. If the binary is not on `PATH`, either fix `PATH` or pass an absolute path: `claude mcp add --scope user csl -- /Users/you/code/bin/csl mcp`.

**First `csl_search` takes tens of seconds**
The first call after a clean install triggers an initial zoekt index build. Subsequent calls reuse the daemon's mmap'd shards and return in hundreds of milliseconds. Pre-warm with `csl search foo` from a terminal before starting Claude Code.

**`csl_search` returns empty for a query that should match**
Run `csl doctor` and check the index state. If a repo is missing or stale, run `csl index --all` or call [`csl_repo_reindex`](#csl_repo_reindex) for the specific repo. Validate the query with [`csl_query_validate`](#csl_query_validate) to rule out a syntax error.

**`csl_repo_lookup` returns empty for a repo that exists**
The checkout is not under any directory listed in `~/.config/csl/config.yaml` `dirs`. Either clone it there or add the parent directory.
