package paperkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackportSkipsChapterWhoseTexMatchesBaseline(t *testing.T) {
	requirePandoc(t)
	root, config := backportFixture(t, "# Chapter\n\nParagraph one.\n")

	results, err := Backport(root, config)
	if err != nil {
		t.Fatalf("Backport() error: %v", err)
	}
	if results[0].Disposition != Untouched {
		t.Errorf("disposition = %v, want %v", results[0].Disposition, Untouched)
	}
	if results[0].Diff != "" {
		t.Errorf("an untouched chapter proposed a diff:\n%s", results[0].Diff)
	}
}

func TestBackportIgnoresTypesettingOnlyEdits(t *testing.T) {
	requirePandoc(t)
	root, config := backportFixture(t, "# Chapter\n\nParagraph one.\n\n![A figure](fig/x.png)\n")

	// Change only the float placement specifier, which markdown cannot
	// represent, so converting back yields identical prose.
	tex := readFile(t, root, "tex/01-chapter.tex")
	edited := strings.Replace(tex, `\begin{figure}`, `\begin{figure}[!t]`, 1)
	if edited == tex {
		t.Skip("generated tex has no figure float to adjust")
	}
	writeFile(t, root, "tex/01-chapter.tex", edited)

	results, err := Backport(root, config)
	if err != nil {
		t.Fatalf("Backport() error: %v", err)
	}
	if results[0].Disposition != TypesetOnly {
		t.Errorf("disposition = %v, want %v", results[0].Disposition, TypesetOnly)
	}
	if results[0].Markdown != nil {
		t.Error("a typesetting-only edit proposed a markdown change")
	}
}

func TestBackportProposesProseChangeAndAppliesOnlyWhenAsked(t *testing.T) {
	requirePandoc(t)
	source := "# Chapter\n\nParagraph one.\n\nParagraph two.\n\nParagraph three.\n"
	root, config := backportFixture(t, source)

	tex := readFile(t, root, "tex/01-chapter.tex")
	edited := strings.Replace(tex, "Paragraph two.", "Paragraph two, corrected in tex.", 1)
	if edited == tex {
		t.Fatal("could not edit the generated tex")
	}
	writeFile(t, root, "tex/01-chapter.tex", edited)

	results, err := Backport(root, config)
	if err != nil {
		t.Fatalf("Backport() error: %v", err)
	}
	if results[0].Disposition != Proposed {
		t.Fatalf("disposition = %v, want %v", results[0].Disposition, Proposed)
	}
	if !strings.Contains(results[0].Diff, "corrected in tex") {
		t.Errorf("diff does not show the prose change:\n%s", results[0].Diff)
	}

	// Backport alone must not touch the working tree.
	if got := readFile(t, root, "01-chapter.md"); got != source {
		t.Errorf("Backport() modified the markdown without confirmation:\n%s", got)
	}

	if err := ApplyBackport(root, results); err != nil {
		t.Fatalf("ApplyBackport() error: %v", err)
	}
	if got := readFile(t, root, "01-chapter.md"); !strings.Contains(got, "corrected in tex") {
		t.Errorf("ApplyBackport() did not apply the change:\n%s", got)
	}
}

func TestBackportReportsUnbasedChapter(t *testing.T) {
	requirePandoc(t)
	root, config := backportFixture(t, "# Chapter\n\nParagraph one.\n")
	if err := os.Remove(baselinePath(root, config, "01-chapter.md")); err != nil {
		t.Fatal(err)
	}

	results, err := Backport(root, config)
	if err != nil {
		t.Fatalf("Backport() error: %v", err)
	}
	if results[0].Disposition != Unbased {
		t.Errorf("disposition = %v, want %v", results[0].Disposition, Unbased)
	}
}

func TestBackportContestsWhenMarkdownAlsoMoved(t *testing.T) {
	requirePandoc(t)
	source := "# Chapter\n\nParagraph one.\n\nParagraph two.\n\nParagraph three.\n"
	root, config := backportFixture(t, source)

	// The same sentence edited on both sides, differently.
	tex := readFile(t, root, "tex/01-chapter.tex")
	writeFile(t, root, "tex/01-chapter.tex",
		strings.Replace(tex, "Paragraph two.", "Paragraph two, from tex.", 1))
	writeFile(t, root, "01-chapter.md",
		strings.Replace(source, "Paragraph two.", "Paragraph two, from markdown.", 1))

	results, err := Backport(root, config)
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("Backport() error = %v, want it to wrap ErrConflicts", err)
	}
	if results[0].Disposition != Contested {
		t.Errorf("disposition = %v, want %v", results[0].Disposition, Contested)
	}
	// A contested chapter is not applied even when the author confirms the
	// batch: its merged text carries conflict markers.
	if err := ApplyBackport(root, results); err != nil {
		t.Fatalf("ApplyBackport() error: %v", err)
	}
	if got := readFile(t, root, "01-chapter.md"); strings.Contains(got, "<<<<<<<") {
		t.Errorf("ApplyBackport() wrote conflict markers into the markdown:\n%s", got)
	}
}

func TestApplyBackportSkipsEverythingButProposals(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "01-chapter.md", "original\n")

	err := ApplyBackport(root, []BackportResult{
		{Chapter: "01-chapter.md", Disposition: Untouched, Markdown: []byte("no\n")},
		{Chapter: "01-chapter.md", Disposition: TypesetOnly, Markdown: []byte("no\n")},
		{Chapter: "01-chapter.md", Disposition: Contested, Markdown: []byte("no\n")},
		{Chapter: "01-chapter.md", Disposition: Unbased, Markdown: []byte("no\n")},
	})
	if err != nil {
		t.Fatalf("ApplyBackport() error: %v", err)
	}
	if got := readFile(t, root, "01-chapter.md"); got != "original\n" {
		t.Errorf("a non-proposal was applied: %q", got)
	}
}

func TestBackportDispositionStringsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for _, disposition := range []BackportDisposition{Untouched, TypesetOnly, Proposed, Contested, Unbased} {
		name := disposition.String()
		if name == "unknown" {
			t.Errorf("disposition %d has no name", disposition)
		}
		if seen[name] {
			t.Errorf("duplicate disposition name %q", name)
		}
		seen[name] = true
	}
}

func TestUnifiedDiffIsEmptyForIdenticalContentAndShowsChanges(t *testing.T) {
	same, err := unifiedDiff("01-chapter.md", []byte("a\n"), []byte("a\n"))
	if err != nil {
		t.Fatalf("unifiedDiff() error: %v", err)
	}
	if same != "" {
		t.Errorf("identical content produced a diff:\n%s", same)
	}

	changed, err := unifiedDiff("01-chapter.md", []byte("a\n"), []byte("b\n"))
	if err != nil {
		t.Fatalf("unifiedDiff() error: %v", err)
	}
	if !strings.Contains(changed, "-a") || !strings.Contains(changed, "+b") {
		t.Errorf("diff does not show the change:\n%s", changed)
	}
}

// backportFixture builds a paper with one chapter and generates its tex, so
// the chapter starts with a baseline that matches.
func backportFixture(t *testing.T, source string) (string, Config) {
	t.Helper()
	root := paperFixture(t, "chapters:\n  - 01-chapter.md\n")
	writeFile(t, root, "01-chapter.md", source)
	writeFile(t, root, filepath.Join("fig", "x.png"), "")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}
	return root, config
}
