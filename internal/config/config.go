// Package config reads and writes lyceum/config.yaml, the committed project
// config (architecture.md, "Config"), and keeps the repo files Lyceum
// depends on in place (EnsureRepoFiles).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// Path is the config file, relative to the repo root.
const Path = "lyceum/config.yaml"

// SchemaVersion is the value of the top-level `lyceum` key this build reads and writes.
const SchemaVersion = 1

// Config is lyceum/config.yaml.
//
// Write keeps every key Read found, including ones not listed here and their
// comments; only the keys below are rewritten, and only when their value changed.
type Config struct {
	Lyceum     int        `yaml:"lyceum"`
	Owner      string     `yaml:"owner"`       // owner identity: git email
	BaseBranch string     `yaml:"base_branch"` // branch units are cut from
	Approvals  Approvals  `yaml:"approvals"`
	Harnesses  []string   `yaml:"harnesses"` // harnesses wired at init
	Hooks      bool       `yaml:"hooks"`     // harness hooks installed
	Understand Understand `yaml:"understand"`

	doc *yaml.Node // the file as read; nil for a new config
}

// Approvals holds the approvals.* keys.
type Approvals struct {
	Signed bool `yaml:"signed"` // require signed approval commits
	// HarnessPrompt allows the harness-prompt approval route where the
	// harness supports it: HarnessPromptAllow or HarnessPromptDeny.
	HarnessPrompt string `yaml:"harness_prompt"`
}

// Values of approvals.harness_prompt.
const (
	HarnessPromptAllow = "allow"
	HarnessPromptDeny  = "deny"
)

// Understand holds the understand.* keys.
type Understand struct {
	WarnThreshold Threshold `yaml:"warn_threshold"` // scope above which understand warns about cost
}

// Threshold is a scope size: warn when either limit is exceeded.
type Threshold struct {
	Files int `yaml:"files"`
	Lines int `yaml:"lines"`
}

// Default returns a new config with the given owner and base branch, which init
// works out from git, and the Config table's defaults for every other key.
func Default(owner, baseBranch string) *Config {
	return &Config{
		Lyceum:     SchemaVersion,
		Owner:      owner,
		BaseBranch: baseBranch,
		Approvals:  Approvals{HarnessPrompt: HarnessPromptAllow},
		Harnesses:  []string{},
		Hooks:      true,
		Understand: Understand{WarnThreshold: Threshold{Files: 2000, Lines: 200000}},
	}
}

// Read reads the config under repoRoot. Keys the file leaves out take their defaults.
func Read(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	return c, nil
}

func parse(data []byte) (*Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("want a mapping of keys to values")
	}
	c := Default("", "")
	c.Lyceum = 0
	if err := doc.Decode(c); err != nil {
		return nil, err
	}
	if c.Lyceum != SchemaVersion {
		return nil, fmt.Errorf("schema version `lyceum: %d` is not supported; this build reads `lyceum: %d`", c.Lyceum, SchemaVersion)
	}
	if h := c.Approvals.HarnessPrompt; h != HarnessPromptAllow && h != HarnessPromptDeny {
		return nil, fmt.Errorf("approvals.harness_prompt is %q; want %s or %s", h, HarnessPromptAllow, HarnessPromptDeny)
	}
	c.doc = &doc
	return c, nil
}

// Write writes c to the config file under repoRoot.
func Write(repoRoot string, c *Config) error {
	data, err := c.marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repoRoot, Path), data, 0o644)
}

func (c *Config) marshal() ([]byte, error) {
	var fresh yaml.Node
	if err := fresh.Encode(c); err != nil {
		return nil, err
	}
	out := &fresh
	if c.doc != nil {
		merge(c.doc.Content[0], &fresh)
		out = c.doc
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// merge sets every key of the mapping src in the mapping dst. Nested mappings are
// merged key by key; any other value replaces dst's unless the two are equal.
// Keys only dst has are kept, and replaced values keep dst's comments.
func merge(dst, src *yaml.Node) {
	for i := 0; i < len(src.Content); i += 2 {
		k, v := src.Content[i], src.Content[i+1]
		j := find(dst, k.Value)
		if j < 0 {
			dst.Content = append(dst.Content, k, v)
			continue
		}
		dv := dst.Content[j+1]
		switch {
		case dv.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode:
			merge(dv, v)
		case same(dv, v):
		default:
			v.HeadComment, v.LineComment, v.FootComment = dv.HeadComment, dv.LineComment, dv.FootComment
			dst.Content[j+1] = v
		}
	}
}

// same reports whether two nodes hold the same value, ignoring style and comments.
func same(a, b *yaml.Node) bool {
	if a.Kind != b.Kind || a.ShortTag() != b.ShortTag() || a.Value != b.Value || len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !same(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

// find returns the index of key in the mapping m, or -1.
func find(m *yaml.Node, key string) int {
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}
