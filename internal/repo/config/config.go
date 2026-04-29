package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"

// Layout constants for session window arrangement.
const (
	LayoutSplit = "split"
	LayoutTab   = "tab"
)

// Config holds the repo finder configuration.
type Config struct {
	Dirs    []string    `yaml:"dirs"`
	Layout  string      `yaml:"layout"`
	Summary bool        `yaml:"summary"`
	TmpDir  string      `yaml:"tmpdir"`
	Hooks   HooksConfig `yaml:"hooks"`
}

// HooksConfig holds configuration for git hooks managed by csl.
type HooksConfig struct {
	PostMerge PostMergeHook `yaml:"post_merge"`
}

// PostMergeHook configures the post-merge hook installer.
// When enabled, `csl hooks install` writes .git/hooks/post-merge into every
// repo discovered via Dirs, except those listed in Exclude.
// Exclude entries are matched against both the repo's absolute path and its
// org/repo name (exact match, no globs).
type PostMergeHook struct {
	Enabled bool     `yaml:"enabled"`
	Exclude []string `yaml:"exclude"`
}

// IsExcluded reports whether the given repo (by absolute path or org/repo name)
// should be skipped by the hook installer.
func (h *PostMergeHook) IsExcluded(repoPath, repoName string) bool {
	if h == nil {
		return false
	}
	for _, e := range h.Exclude {
		e = expandTilde(e)
		if e == repoPath || e == repoName {
			return true
		}
	}
	return false
}

// SummaryEnabled returns true when the summary tab should be created.
// Requires summary: true AND layout: tab.
func (c *Config) SummaryEnabled() bool {
	return c != nil && c.Summary && c.EffectiveLayout() == LayoutTab
}

// EffectiveTmpDir returns the configured tmpdir for scratch sessions.
// Returns empty string when unset, meaning os.MkdirTemp default should be used.
func (c *Config) EffectiveTmpDir() string {
	if c != nil && c.TmpDir != "" {
		return expandTilde(c.TmpDir)
	}
	return ""
}

// EffectiveLayout returns the configured layout, defaulting to split.
// Safe to call on a nil receiver.
func (c *Config) EffectiveLayout() string {
	if c != nil && c.Layout == LayoutTab {
		return LayoutTab
	}
	return LayoutSplit
}

// Load reads config.yaml from ~/.config/csl/config.yaml.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	globalPath := filepath.Join(home, ".config", "csl", configFileName)
	cfg, err := loadFrom(globalPath)
	if err != nil {
		return nil, fmt.Errorf("no config found (checked %s): %w", globalPath, err)
	}
	return cfg, nil
}

// LoadFrom reads config from a specific path.
func LoadFrom(path string) (*Config, error) {
	return loadFrom(path)
}

func loadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Expand tildes in directory paths
	for i, d := range cfg.Dirs {
		cfg.Dirs[i] = expandTilde(d)
	}
	cfg.TmpDir = expandTilde(cfg.TmpDir)
	for i, e := range cfg.Hooks.PostMerge.Exclude {
		cfg.Hooks.PostMerge.Exclude[i] = expandTilde(e)
	}

	return &cfg, nil
}

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
