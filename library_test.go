package paperkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// libraryFixture writes a paper that converts through md-to-tex: the engine,
// a reference corpus inside the paper directory so the test does not reach
// outside its own root, a title page, and whatever chapters the case needs.
func libraryFixture(t *testing.T, chapters ...string) string {
	t.Helper()
	config := "engine: md-to-tex\nbibliography: references.yaml\ntitle_page: 00-front-matter.md\nchapters:\n"
	for _, chapter := range chapters {
		config += "  - " + chapter + "\n"
	}
	root := paperFixture(t, config)
	writeFile(t, root, "references.yaml", "- id: lee-2026\n  title: A Cited Work\n")
	writeFile(t, root, "00-front-matter.md",
		"---\ntitle: A Paper\nauthor:\n  - Petar Djukic\nabstract: The abstract.\n---\n")
	return root
}

// poisonPandoc puts a pandoc on PATH that fails whenever it is run, so a test
// that succeeds proves the path under test never called it. Asserting on the
// absence of a subprocess needs a witness; this is the cheapest one.
func poisonPandoc(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'pandoc must not run on the library path' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "pandoc"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGenerateTexLibraryEngineConvertsWithoutPandoc(t *testing.T) {
	poisonPandoc(t)
	root := libraryFixture(t, "01-first.md", "02-second.md")
	writeFile(t, root, "01-first.md", "# First\n\nProse citing [@lee-2026].\n")
	writeFile(t, root, "02-second.md", "# Second\n\nMore prose.\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatalf("GenerateTex() error: %v", err)
	}

	first := readFile(t, root, "tex/01-first.tex")
	if !strings.Contains(first, `\section{First}`) {
		t.Errorf("01-first.tex has no section heading:\n%s", first)
	}
	if !strings.Contains(first, `\cite{lee-2026}`) {
		t.Errorf("01-first.tex did not render the citation:\n%s", first)
	}
	if strings.Contains(first, `\documentclass`) {
		t.Errorf("01-first.tex is standalone, want a fragment:\n%s", first)
	}

	// The title page becomes a fragment of its own rather than being spliced
	// into the container, which is what makes its name match its markdown.
	front := readFile(t, root, "tex/00-front-matter.tex")
	if !strings.Contains(front, `\title{A Paper}`) || !strings.Contains(front, `\begin{abstract}`) {
		t.Errorf("00-front-matter.tex carries no title block or abstract:\n%s", front)
	}
}

func TestGenerateTexLibraryContainerInputsRosterInOrder(t *testing.T) {
	poisonPandoc(t)
	root := libraryFixture(t, "02-second.md", "01-first.md")
	writeFile(t, root, "01-first.md", "# First\n")
	writeFile(t, root, "02-second.md", "# Second\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatalf("GenerateTex() error: %v", err)
	}

	main := readFile(t, root, "tex/main.tex")
	for _, want := range []string{
		`\documentclass[journal]{IEEEtran}`,
		`\input{../templates/ieee-preamble}`,
		`\bibliographystyle{IEEEtranN}`,
		`\graphicspath{{../}{../fig/}}`,
		`\bibliography{references}`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tex has no %s:\n%s", want, main)
		}
	}

	// Config order is authoritative, and the title page leads.
	order := []string{`\input{00-front-matter}`, `\input{02-second}`, `\input{01-first}`}
	position := 0
	for _, input := range order {
		index := strings.Index(main[position:], input)
		if index < 0 {
			t.Fatalf("main.tex does not input %s after the previous one:\n%s", input, main)
		}
		position += index
	}
	// The container holds no content of its own.
	if strings.Contains(main, `\section{`) {
		t.Errorf("main.tex carries chapter content:\n%s", main)
	}
}

func TestGenerateTexLibraryReportsLabelCollisions(t *testing.T) {
	poisonPandoc(t)
	root := libraryFixture(t, "01-first.md", "02-second.md")
	writeFile(t, root, "01-first.md", "# First\n\n## Level assessment\n\nProse.\n")
	writeFile(t, root, "02-second.md", "# Second\n\n## Level assessment\n\nProse.\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = GenerateTex(root, config)
	if !errors.Is(err, ErrCollisions) {
		t.Fatalf("GenerateTex() error = %v, want ErrCollisions", err)
	}
	for _, want := range []string{"level-assessment", "01-first.md", "02-second.md", "Level assessment"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("collision error does not name %q:\n%v", want, err)
		}
	}

	// Nothing was written: a collision is settled in the markdown, and a
	// half-regenerated tex directory would leave the author reconciling
	// fragments that were never meant to land.
	if _, err := os.Stat(filepath.Join(root, "tex", "01-first.tex")); !os.IsNotExist(err) {
		t.Errorf("a fragment was written despite the collision")
	}
}

// A label the author states in two chapters collides exactly like two derived
// ones, and neither is renamed to resolve it.
func TestGenerateTexLibraryNeverPrefixesLabels(t *testing.T) {
	poisonPandoc(t)
	root := libraryFixture(t, "01-first.md")
	writeFile(t, root, "01-first.md", "# First\n\n## Background\n\nProse.\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatalf("GenerateTex() error: %v", err)
	}

	first := readFile(t, root, "tex/01-first.tex")
	if !strings.Contains(first, `\label{background}`) {
		t.Errorf("01-first.tex has no derived label:\n%s", first)
	}
	if strings.Contains(first, `\label{01-first:`) {
		t.Errorf("01-first.tex carries a chapter-prefixed label, which breaks cross-chapter references:\n%s", first)
	}
}

func TestGenerateTexLibraryRejectsUnknownCitationKey(t *testing.T) {
	poisonPandoc(t)
	root := libraryFixture(t, "01-first.md")
	writeFile(t, root, "01-first.md", "# First\n\nProse citing [@not-in-the-corpus].\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = GenerateTex(root, config)
	if err == nil {
		t.Fatal("GenerateTex() accepted a citation the corpus does not define")
	}
	for _, want := range []string{"01-first.md", "not-in-the-corpus"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

func TestCitationKeysReadsCorpusIDs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "references.yaml", "- id: first-2026\n  title: One\n- id: second-2026\n  title: Two\n")

	keys, err := citationKeys(root, Config{Bibliography: "references.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "first-2026" || keys[1] != "second-2026" {
		t.Errorf("citationKeys() = %v, want both corpus ids", keys)
	}
}

func TestCitationKeysRejectsEmptyCorpus(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "references.yaml", "[]\n")

	if _, err := citationKeys(root, Config{Bibliography: "references.yaml"}); err == nil {
		t.Error("citationKeys() accepted a corpus defining no ids")
	}
}
