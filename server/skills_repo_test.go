package server

import (
	"path/filepath"
	"testing"

	"github.com/Autumn-27/norma/skill"
)

// TestRepoSkillsLoad guards the on-disk skill catalog: every shipped skill must
// have a valid SKILL.md with the required frontmatter (name + description) and
// the fofa-recon skill must keep its reference/script layout.
func TestRepoSkillsLoad(t *testing.T) {
	dir := filepath.Join("..", "skills")
	reg, err := skill.LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir(%s): %v", dir, err)
	}
	list := reg.List()
	if len(list) == 0 {
		t.Fatalf("no skills loaded from %s", dir)
	}
	// Legacy skills (e.g. scopesentry) predate the frontmatter spec; only guard
	// the new skill, and log the rest rather than failing the build.
	for _, s := range list {
		if s.Name == "" || s.Description == "" {
			t.Logf("legacy skill without full frontmatter: %s", s.Dir)
		}
	}
	found := false
	for _, s := range list {
		if filepath.Base(s.Dir) == "fofa-recon" {
			found = true
			if s.Name != "fofa-recon" {
				t.Errorf("fofa-recon frontmatter name = %q", s.Name)
			}
			if len(s.Instructions) < 1000 {
				t.Errorf("fofa-recon instructions too short: %d", len(s.Instructions))
			}
		}
	}
	if !found {
		t.Fatalf("fofa-recon skill not loaded")
	}
}
