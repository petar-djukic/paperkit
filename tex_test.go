package paperkit

import (
	"os"
	"path/filepath"
	"testing"
)

// The conversion itself is covered in library_test.go; this file holds the
// engine-independent behavior of GenerateTex.

func TestGenerateTexAcceptsRelativeRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	// No engine line: a paper that names none converts through md-to-tex,
	// which is the only engine since the pandoc path was removed (GH-588).
	writeFile(t, root, "paper.yaml", "bibliography: references.yaml\nchapters:\n  - 01-first.md\n")
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"), "% preamble\n")
	writeFile(t, root, "references.yaml", "- id: lee-2026\n  title: A Cited Work\n")
	writeFile(t, root, "01-first.md", "# First\n")

	t.Chdir(parent)
	config, err := LoadConfig("paper")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex("paper", config); err != nil {
		t.Fatalf("GenerateTex() with a relative root: %v", err)
	}
	for _, name := range []string{"paper/tex/01-first.tex", "paper/tex/main.tex"} {
		if _, err := os.Stat(filepath.FromSlash(name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
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
