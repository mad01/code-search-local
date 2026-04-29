package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/code-search-local/internal/repo/config"
	"github.com/mad01/code-search-local/internal/repo/finder"
)

// hookMarker identifies hooks managed by csl. Bumping the version here lets a
// future `csl hooks install` recognize and overwrite older managed hooks.
const hookMarker = "# csl-managed-hook: post-merge v1"

const hookScript = `#!/usr/bin/env sh
` + hookMarker + `
# Edit csl config (~/.config/csl/config.yaml), not this file —
# ` + "`csl hooks install`" + ` will overwrite it.
csl index --repo "$(git rev-parse --show-toplevel)" >/dev/null 2>&1 &
disown 2>/dev/null || true
exit 0
`

var (
	hooksDryRunFlag bool
	hooksForceFlag  bool
	hooksRepoFlag   string
	hooksJSONFlag   bool
)

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage csl-managed git hooks (post-merge auto-reindex)",
	Long: `Manage git hooks that csl writes into local repos.

When the post_merge hook is enabled in ~/.config/csl/config.yaml, ` + "`csl hooks install`" + `
writes a .git/hooks/post-merge script into every discovered repo (except those
in hooks.post_merge.exclude). The hook runs ` + "`csl index --repo <path>`" + ` after
each git pull, keeping the search index fresh without manual reindex calls.

Hooks live outside version control (in .git/hooks/) so they're per-checkout.
Re-running ` + "`csl hooks install`" + ` is idempotent — it overwrites csl-managed hooks
in place but refuses to clobber foreign hooks unless --force is passed.`,
}

var hooksInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install or update post-merge hook in every non-excluded repo",
	RunE:  runHooksInstall,
}

var hooksUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove csl-managed post-merge hook from every repo",
	RunE:  runHooksUninstall,
}

var hooksStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hook installation state for every repo",
	RunE:  runHooksStatus,
}

func init() {
	hooksInstallCmd.Flags().BoolVar(&hooksDryRunFlag, "dry-run", false, "print planned changes without writing files")
	hooksInstallCmd.Flags().BoolVar(&hooksForceFlag, "force", false, "overwrite even foreign (non-csl-managed) hooks")
	hooksInstallCmd.Flags().StringVar(&hooksRepoFlag, "repo", "", "operate on a single repo by absolute path (debugging)")
	hooksUninstallCmd.Flags().StringVar(&hooksRepoFlag, "repo", "", "operate on a single repo by absolute path (debugging)")
	hooksUninstallCmd.Flags().BoolVar(&hooksDryRunFlag, "dry-run", false, "print planned changes without writing files")
	hooksStatusCmd.Flags().BoolVar(&hooksJSONFlag, "json", false, "output as JSON")

	hooksCmd.AddCommand(hooksInstallCmd, hooksUninstallCmd, hooksStatusCmd)
	rootCmd.AddCommand(hooksCmd)
}

// hookState describes the state of the post-merge hook in a single repo.
type hookState struct {
	Repo     string `json:"repo"`
	Path     string `json:"path"`
	Status   string `json:"status"`             // installed | missing | foreign | excluded | size_excluded
	Excluded bool   `json:"excluded"`
	Reason   string `json:"reason,omitempty"`
}

// loadReposForHooks loads config + walks repos, honoring --repo if set.
// Returns the configured PostMergeHook so callers can apply exclusions.
func loadReposForHooks() ([]finder.Repo, *config.PostMergeHook, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	if hooksRepoFlag != "" {
		abs, err := filepath.Abs(hooksRepoFlag)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve --repo path: %w", err)
		}
		repo, err := finder.Inspect(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect %s: %w", abs, err)
		}
		return []finder.Repo{repo}, &cfg.Hooks.PostMerge, nil
	}

	repos, err := finder.Walk(cfg.Dirs)
	if err != nil {
		return nil, nil, err
	}
	return repos, &cfg.Hooks.PostMerge, nil
}

func runHooksInstall(cmd *cobra.Command, args []string) error {
	repos, postMerge, err := loadReposForHooks()
	if err != nil {
		return err
	}

	if !postMerge.Enabled && hooksRepoFlag == "" {
		return fmt.Errorf("hooks.post_merge.enabled is false in config — nothing to install")
	}

	w := cmd.OutOrStdout()
	var changed, skipped, foreign, errored int

	for _, repo := range repos {
		if postMerge.IsExcluded(repo.Path, repo.Name) {
			fmt.Fprintf(w, "  skip %s — excluded by config\n", repo.Name)
			skipped++
			continue
		}

		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		existing, readErr := os.ReadFile(hookPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			fmt.Fprintf(w, "  ERR  %s — read %s: %v\n", repo.Name, hookPath, readErr)
			errored++
			continue
		}

		isForeign := readErr == nil && !strings.Contains(string(existing), hookMarker)
		if isForeign && !hooksForceFlag {
			fmt.Fprintf(w, "  skip %s — foreign hook present (use --force to overwrite)\n", repo.Name)
			foreign++
			continue
		}

		if string(existing) == hookScript {
			continue
		}

		if hooksDryRunFlag {
			fmt.Fprintf(w, "  would write %s\n", hookPath)
			changed++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
			fmt.Fprintf(w, "  ERR  %s — mkdir: %v\n", repo.Name, err)
			errored++
			continue
		}
		if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
			fmt.Fprintf(w, "  ERR  %s — write: %v\n", repo.Name, err)
			errored++
			continue
		}
		fmt.Fprintf(w, "  wrote %s\n", repo.Name)
		changed++
	}

	fmt.Fprintf(w, "\n%d changed, %d excluded, %d foreign, %d errors\n", changed, skipped, foreign, errored)
	if errored > 0 {
		return fmt.Errorf("%d repo(s) failed", errored)
	}
	return nil
}

func runHooksUninstall(cmd *cobra.Command, args []string) error {
	repos, _, err := loadReposForHooks()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	var removed, skipped, foreign int

	for _, repo := range repos {
		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		existing, readErr := os.ReadFile(hookPath)
		if os.IsNotExist(readErr) {
			skipped++
			continue
		}
		if readErr != nil {
			fmt.Fprintf(w, "  ERR  %s — read %s: %v\n", repo.Name, hookPath, readErr)
			continue
		}
		if !strings.Contains(string(existing), hookMarker) {
			fmt.Fprintf(w, "  skip %s — foreign hook (not csl-managed)\n", repo.Name)
			foreign++
			continue
		}
		if hooksDryRunFlag {
			fmt.Fprintf(w, "  would remove %s\n", hookPath)
			removed++
			continue
		}
		if err := os.Remove(hookPath); err != nil {
			fmt.Fprintf(w, "  ERR  %s — remove: %v\n", repo.Name, err)
			continue
		}
		fmt.Fprintf(w, "  removed %s\n", repo.Name)
		removed++
	}

	fmt.Fprintf(w, "\n%d removed, %d had no hook, %d foreign\n", removed, skipped, foreign)
	return nil
}

func runHooksStatus(cmd *cobra.Command, args []string) error {
	repos, postMerge, err := loadReposForHooks()
	if err != nil {
		return err
	}

	states := make([]hookState, 0, len(repos))
	for _, repo := range repos {
		s := hookState{Repo: repo.Name, Path: repo.Path}
		if postMerge.IsExcluded(repo.Path, repo.Name) {
			s.Status = "excluded"
			s.Excluded = true
			s.Reason = "matched config exclude"
			states = append(states, s)
			continue
		}

		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		data, readErr := os.ReadFile(hookPath)
		switch {
		case os.IsNotExist(readErr):
			s.Status = "missing"
		case readErr != nil:
			s.Status = "error"
			s.Reason = readErr.Error()
		case strings.Contains(string(data), hookMarker):
			s.Status = "installed"
		default:
			s.Status = "foreign"
		}
		states = append(states, s)
	}

	w := cmd.OutOrStdout()
	if hooksJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(states)
	}

	fmt.Fprintf(w, "%-50s %-12s %s\n", "REPO", "HOOK", "NOTE")
	for _, s := range states {
		note := s.Reason
		fmt.Fprintf(w, "%-50s %-12s %s\n", s.Repo, s.Status, note)
	}
	return nil
}
