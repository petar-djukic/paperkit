package paperkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	root := paperFixture(t, "chapters:\n  - 01-intro.md\n", "01-intro.md")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	for _, field := range []struct {
		name string
		got  string
		want string
	}{
		{"FigureDir", config.FigureDir, "fig"},
		{"TexDir", config.TexDir, "tex"},
		{"BuildDir", config.BuildDir, "build"},
		{"Output", config.Output, "paper.pdf"},
		{"Preamble", config.Preamble, filepath.Join("templates", "ieee-preamble.tex")},
	} {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
}

func TestLoadConfigKeepsExplicitValues(t *testing.T) {
	root := paperFixture(t, `title: Explicit
chapters:
  - 01-intro.md
figure_dir: diagrams
tex_dir: latex
build_dir: out
output: article.pdf
preamble: templates/ieee-preamble.tex
`, "01-intro.md")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if config.Title != "Explicit" || config.FigureDir != "diagrams" ||
		config.TexDir != "latex" || config.BuildDir != "out" || config.Output != "article.pdf" {
		t.Errorf("explicit values were not preserved: %+v", config)
	}
}

func TestLoadConfigPreservesChapterOrder(t *testing.T) {
	root := paperFixture(t,
		"chapters:\n  - 02-second.md\n  - 01-first.md\n",
		"01-first.md", "02-second.md")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	want := []string{"02-second.md", "01-first.md"}
	for index, chapter := range want {
		if config.Chapters[index] != chapter {
			t.Errorf("Chapters[%d] = %q, want %q", index, config.Chapters[index], chapter)
		}
	}
}

func TestLoadConfigRejectsMissingChapter(t *testing.T) {
	root := paperFixture(t, "chapters:\n  - 01-intro.md\n  - 02-gone.md\n", "01-intro.md")

	_, err := LoadConfig(root)
	if err == nil {
		t.Fatal("LoadConfig() unexpectedly passed")
	}
	if !strings.Contains(err.Error(), "02-gone.md") {
		t.Errorf("error %q does not name the missing chapter", err)
	}
}

func TestLoadConfigRejectsEmptyAndDuplicateChapters(t *testing.T) {
	empty := paperFixture(t, "title: No Chapters\n")
	if _, err := LoadConfig(empty); err == nil || !strings.Contains(err.Error(), "no chapters") {
		t.Errorf("empty chapter list error = %v", err)
	}

	duplicate := paperFixture(t, "chapters:\n  - 01-intro.md\n  - 01-intro.md\n", "01-intro.md")
	if _, err := LoadConfig(duplicate); err == nil || !strings.Contains(err.Error(), "listed twice") {
		t.Errorf("duplicate chapter error = %v", err)
	}
}

func TestLoadConfigRejectsMissingPreamble(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "paper.yaml", "chapters:\n  - 01-intro.md\n")
	writeFile(t, root, "01-intro.md", "# Intro\n")

	_, err := LoadConfig(root)
	if err == nil || !strings.Contains(err.Error(), "preamble") {
		t.Errorf("missing preamble error = %v", err)
	}
}

func TestLoadConfigReportsUnreadableAndMalformed(t *testing.T) {
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Error("LoadConfig() on a directory with no paper.yaml unexpectedly passed")
	}

	malformed := t.TempDir()
	writeFile(t, malformed, "paper.yaml", "chapters: [unclosed\n")
	if _, err := LoadConfig(malformed); err == nil {
		t.Error("LoadConfig() on malformed YAML unexpectedly passed")
	}
}

func TestTexNameKeepsStem(t *testing.T) {
	for chapter, want := range map[string]string{
		"01-introduction.md": "01-introduction.tex",
		"10-appendix-a.md":   "10-appendix-a.tex",
		"sub/02-agent.md":    "02-agent.tex",
		"00-title-page.text": "00-title-page.tex",
	} {
		if got := TexName(chapter); got != want {
			t.Errorf("TexName(%q) = %q, want %q", chapter, got, want)
		}
	}
}

// paperFixture writes a paper.yaml, the named chapter files, and the default
// preamble into a fresh directory.
func paperFixture(t *testing.T, config string, chapters ...string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "paper.yaml", config)
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"), "% preamble\n")
	for _, chapter := range chapters {
		writeFile(t, root, chapter, "# "+chapter+"\n")
	}
	return root
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
