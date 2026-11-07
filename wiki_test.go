package paperkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePandoc puts a pandoc on PATH that wraps its input file in a paragraph,
// so the wiki's assembly is testable without the real converter.
func fakePandoc(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nfor last; do :; done\nprintf '<p>%s</p>\\n' \"$(cat \"$last\")\"\n"
	if err := os.WriteFile(filepath.Join(dir, "pandoc"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func wikiFixture(t *testing.T) (string, Config) {
	t.Helper()
	root, config := buildFixture(t)
	writeFile(t, root, filepath.Join("templates", "wiki.html"), "<main>{{LANGUAGE}}</main><aside>{{PAPERS}}</aside>\n")
	writeFile(t, root, "wiki.md", "the language")
	writeFile(t, root, "papers.md", "the papers")
	config.Wiki = WikiConfig{
		Template: filepath.Join("templates", "wiki.html"),
		Sections: map[string]string{"LANGUAGE": "wiki.md", "PAPERS": "papers.md"},
		Output:   filepath.Join("build", "wiki.html"),
	}
	return root, config
}

func TestWikiFillsEachPlaceholderFromItsSection(t *testing.T) {
	fakePandoc(t)
	root, config := wikiFixture(t)

	if err := Wiki(root, config); err != nil {
		t.Fatalf("Wiki() error: %v", err)
	}
	got := readFile(t, root, filepath.Join("build", "wiki.html"))
	want := "<main><p>the language</p>\n</main><aside><p>the papers</p>\n</aside>\n"
	if got != want {
		t.Errorf("wiki = %q, want %q", got, want)
	}
}

func TestWikiRejectsASectionWithNoPlaceholder(t *testing.T) {
	fakePandoc(t)
	root, config := wikiFixture(t)
	config.Wiki.Sections["EXTRA"] = "papers.md"

	if err := Wiki(root, config); err == nil || !strings.Contains(err.Error(), "{{EXTRA}}") {
		t.Errorf("Wiki() = %v, want it to name the missing placeholder", err)
	}
}

func TestWikiRequiresConfiguration(t *testing.T) {
	root, config := buildFixture(t)
	if err := Wiki(root, config); err == nil {
		t.Error("Wiki() with no wiki configured succeeded")
	}
}

func TestInlineFiguresEmbedsImagesUnderTheDiagramsDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, filepath.Join("figures", "a.png"), "PNG")
	writeFile(t, root, filepath.Join("figures", "b.svg"),
		"<svg width=\"1\">\n<rect x=\"0\" y=\"0\" width=\"10\" height=\"10\" fill=\"#f6f7f5\"/>\n<path stroke=\"#20262c\"/></svg>")
	config := Config{Diagrams: "figures"}

	got, err := inlineFigures(root, config, `<img src="figures/a.png" alt="a"><img src="figures/b.svg" alt="b"><img src="elsewhere/c.png">`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<img class="diagram" src="data:image/png;base64,UE5H" alt="a">`,
		`<svg class="fig" width="1">`,
		`stroke="currentColor"`,
		`<img src="elsewhere/c.png">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inlined HTML lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "#f6f7f5") {
		t.Errorf("the SVG kept its fixed ground:\n%s", got)
	}
}
