# Hooks

`csl hooks` writes a `post-merge` git hook into every repo `csl` discovers, so the search index is refreshed automatically after each `git pull`. Without it, the index goes stale until the next `csl search` (or until a repo's fingerprint check trips on the next `csl index` run).

The installer is opt-in via `~/.config/csl/config.yaml` and re-running it is a no-op when nothing has changed, so it is safe to wire into `ralph apply` or any other config-bootstrap step.

## When to use it

Turn it on if you `git pull` more often than you run `csl search` and want hits to reflect the latest code without thinking about it. Skip it (or exclude specific repos) when:

- A repo is large enough that a full re-index after every pull is too slow.
- A repo is rarely searched and the staleness check on the next `csl search` is good enough.
- A repo already has a project-specific `post-merge` hook (csl will refuse to overwrite it; see [`csl hooks install`](#csl-hooks-install)).

## Configure

Hooks are configured in `~/.config/csl/config.yaml` under a `hooks` block. The block is optional; if it is missing or `enabled: false`, `csl hooks install` refuses to run with `hooks.post_merge.enabled is false in config — nothing to install`.

### Schema

| Field | Type | Default | Description |
|---|---|---|---|
| `hooks.post_merge.enabled` | bool | `false` | Master switch for the installer. `csl hooks install` is a no-op when false. |
| `hooks.post_merge.exclude` | list of strings | `[]` | Repos to skip. Each entry matches against the repo's absolute path *or* its `org/repo` name (exact match, no globs). Tildes are expanded. |

### Example

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace

hooks:
  post_merge:
    enabled: true
    exclude:
      - ~/workspace/services-pilot
      - myorg/big-monorepo
```

### Exclusion semantics

Each entry in `exclude` is matched twice for every discovered repo: once against the absolute filesystem path, once against the `org/repo` name extracted from the origin remote. Either match excludes the repo. There are no globs and no regex.

Use a path entry when the org/repo name is ambiguous or when the repo has no remote. Use a name entry when the same repo could live under different parent directories on different machines.

`csl hooks status` reports the exclusion explicitly:

```
spotify/services-pilot                   excluded     matched config exclude
```

## `csl hooks install`

Install or update the `post-merge` hook in every non-excluded repo.

### Synopsis

```sh
csl hooks install
csl hooks install --dry-run
csl hooks install --repo <path>
csl hooks install --force
```

### Description

`csl hooks install` walks the `dirs` from your config, applies `hooks.post_merge.exclude`, and for every remaining repo writes `<repo>/.git/hooks/post-merge` with the script shown below in [The post-merge hook script](#the-post-merge-hook-script).

The hook file is identified as csl-managed by a marker line near the top:

```sh
# csl-managed-hook: post-merge v1
```

Re-running `install` compares the on-disk file byte-for-byte with the script that would be written. Identical files are skipped, so repeated runs cost only stat + read calls. Bumping the version number in the marker (currently `v1`) is how a future csl release would force a rewrite of older managed hooks.

**Foreign hooks.** If `<repo>/.git/hooks/post-merge` exists and does not contain the marker, csl assumes another tool wrote it (commonly `git-lfs`) and refuses to overwrite. The repo is reported as `foreign` in the run summary and in `csl hooks status`. Pass `--force` to overwrite anyway. There is no merge mode.

`install` writes the file with mode `0755`. The parent `.git/hooks/` directory is created if missing.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Print planned writes without touching the filesystem. |
| `--force` | `false` | Overwrite foreign (non-csl-managed) hooks. |
| `--repo <path>` | `""` | Operate on a single repo by absolute path. Skips the `dirs` walk and the `enabled` check. Useful for debugging or for a one-off install on a repo outside the configured tree. |

### Examples

**Preview before writing:**

```sh
csl hooks install --dry-run
```

Output:

```
  would write /Users/you/code/src/github.com/myorg/foo/.git/hooks/post-merge
  would write /Users/you/code/src/github.com/myorg/bar/.git/hooks/post-merge
  skip /Users/you/workspace/services-pilot — excluded by config
  skip /Users/you/code/src/github.com/myorg/dotfiles — foreign hook present (use --force to overwrite)

2 changed, 1 excluded, 1 foreign, 0 errors
```

**Install for real:**

```sh
csl hooks install
```

**Install on one repo without touching the rest:**

```sh
csl hooks install --repo ~/code/src/github.com/myorg/foo
```

**Force-overwrite a foreign hook:**

```sh
csl hooks install --force --repo ~/code/src/github.com/myorg/dotfiles
```

`install` exits non-zero only if at least one repo failed with an I/O error. A repo being foreign or excluded is not an error.

## `csl hooks uninstall`

Remove the csl-managed `post-merge` hook from every repo.

### Synopsis

```sh
csl hooks uninstall
csl hooks uninstall --dry-run
csl hooks uninstall --repo <path>
```

### Description

`uninstall` walks the same `dirs` as `install`, but instead of writing it deletes any `post-merge` hook that contains the csl marker. Foreign hooks (no marker) are left in place and reported.

There is no `--force` for uninstall: csl will not delete a hook it didn't write.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Print planned removals without touching the filesystem. |
| `--repo <path>` | `""` | Operate on a single repo by absolute path. |

## `csl hooks status`

Show hook installation state for every discovered repo.

### Synopsis

```sh
csl hooks status
csl hooks status --json
```

### Description

`status` reports one row per repo. The `HOOK` column takes one of:

| Value | Meaning |
|---|---|
| `installed` | csl-managed hook present (marker found). |
| `missing` | No `post-merge` hook in `.git/hooks/`. |
| `foreign` | A `post-merge` hook exists but doesn't contain the csl marker. `install` will skip; `--force` would overwrite. |
| `excluded` | Repo matches a `hooks.post_merge.exclude` entry. |
| `error` | The hook file couldn't be read. The cause is in the `NOTE` column. |

### Example output

```
REPO                                               HOOK         NOTE
myorg/foo                                          installed
myorg/bar                                          missing
myorg/dotfiles                                     foreign
spotify/services-pilot                             excluded     matched config exclude
```

JSON output is one object per repo with fields `repo`, `path`, `status`, `excluded`, `reason`.

## The post-merge hook script

Every csl-managed `post-merge` hook is exactly this:

```sh
#!/usr/bin/env sh
# csl-managed-hook: post-merge v1
# Edit csl config (~/.config/csl/config.yaml), not this file —
# `csl hooks install` will overwrite it.
csl index --repo "$(git rev-parse --show-toplevel)" >/dev/null 2>&1 &
disown 2>/dev/null || true
exit 0
```

A few notes on the choices:

- The reindex is backgrounded and `disown`ed so `git pull` returns immediately. The reindex runs detached; you do not wait for it.
- Both stdout and stderr are redirected to `/dev/null`. Any failure mode (csl missing from `PATH`, fingerprint error, shard write error) is silent on the console. Check `~/.config/csl/search-daemon.log` if a repo's index drifts.
- `git rev-parse --show-toplevel` resolves the repo root from inside the hook regardless of where in the worktree the merge ran.
- The marker line is the contract. Bumping `v1` is how a future csl release would force a rewrite of older managed hooks.

## Single-repo reindex (`csl index --repo`)

The hook calls `csl index --repo <path>`, which is the only way to reindex one repo without scanning every other configured directory. It is also useful from the terminal:

```sh
csl index --repo ~/code/src/github.com/myorg/foo
```

The flag bypasses the global staleness check, calls [`finder.Inspect`](../internal/repo/finder/walker.go) to resolve the repo's name and remote, and updates the per-repo entry in `state.json` so the next `csl index` run won't redundantly re-process it. See [CLI reference](cli.md#csl-index) for the full `csl index` flag list.

## Re-applying with ralph

`install` is safe to call on every `ralph apply`. The byte-for-byte compare described in [`csl hooks install`](#csl-hooks-install) means an unchanged hook costs only a stat + read. Wire it in via the recipe's `[hooks]` block:

```toml
[hooks]
post_apply = ["csl hooks install || true"]
```

The `|| true` keeps `ralph apply` green when csl isn't installed yet (for example, on a freshly bootstrapped machine where the csl binary hasn't been built).
