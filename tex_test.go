package paperkit

import (
	"os"
	"path/filepath"
	"testing"
)

// The conversion itself is covered in library_test.go; this file holds what
// GenerateTex does with the converted fragments.

func TestGenerateTexAcceptsRelativeRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	writeFile(t, root, "paper.yaml", "bibliography: references.yaml\nchapters:\n  - 01-first.md\n")
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"), "% preamble\n")
	writeFile(t, root, "references.yaml", "- id: lee-2026\n  title: A Cited Work\n")
	writeFile(t, root, "01-first.md", "# First\n")

	t.Chdir(parent)
	config, err := LoadConfig("paper")
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateTex("paper", config); err != nil {
		t.Fatalf("GenerateTex() with a relative root: %v", err)
	}
	for _, name := range []string{"paper/tex/01-first.tex", "paper/tex/main.tex"} {
		if _, err := os.Stat(filepath.FromSlash(name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}
}

// tex/ is build output, so a second run reproduces the first exactly and
// leaves no record of earlier runs behind.
func TestGenerateTexIsStableAndKeepsNoBaseline(t *testing.T) {
	root := paperFixture(t, "bibliography: references.yaml\nchapters:\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n\nProse.\n")
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, root, "tex/01-first.tex")
	if err := GenerateTex(root, config); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if after := readFile(t, root, "tex/01-first.tex"); after != before {
		t.Error("a no-change regeneration altered the tex")
	}
	if _, err := os.Stat(filepath.Join(root, "tex", ".baseline")); err == nil {
		t.Error("GenerateTex created a .baseline directory")
	}
}

// A fragment the author edited is overwritten: markdown is the only source.
func TestGenerateTexOverwritesEditedFragment(t *testing.T) {
	root := paperFixture(t, "bibliography: references.yaml\nchapters:\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n\nProse.\n")
	writeFile(t, root, "tex/01-first.tex", "hand-edited\n")
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "tex/01-first.tex"); got == "hand-edited\n" {
		t.Error("GenerateTex kept a hand-edited fragment instead of regenerating it")
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
