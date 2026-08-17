package paperkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A chapter long enough that an edit near the top and one near the bottom are
// separate hunks, which is the realistic shape: the author adjusts a float in
// one place while prose changes somewhere else.
const chapterBody = `# Chapter

Paragraph one.

Paragraph two.

Paragraph three.

Paragraph four.

Paragraph five.

Paragraph six.
`

func TestReconcileCreatesWhenNoTexExists(t *testing.T) {
	root, config := mergeFixture(t)

	disposition, err := reconcile(root, config, "01-chapter.md", []byte("GENERATED\n"))
	if err != nil {
		t.Fatalf("reconcile() error: %v", err)
	}
	if disposition != Created {
		t.Errorf("disposition = %v, want %v", disposition, Created)
	}
	if got := readFile(t, root, "tex/01-chapter.tex"); got != "GENERATED\n" {
		t.Errorf("tex = %q, want the generated content", got)
	}
	if got := readFile(t, root, "tex/.baseline/01-chapter.tex"); got != "GENERATED\n" {
		t.Errorf("baseline = %q, want the generated content", got)
	}
}

func TestReconcileReplacesWhenTexMatchesBaseline(t *testing.T) {
	root, config := mergeFixture(t)
	writeFile(t, root, "tex/01-chapter.tex", "OLD\n")
	writeFile(t, root, "tex/.baseline/01-chapter.tex", "OLD\n")

	disposition, err := reconcile(root, config, "01-chapter.md", []byte("NEW\n"))
	if err != nil {
		t.Fatalf("reconcile() error: %v", err)
	}
	if disposition != Replaced {
		t.Errorf("disposition = %v, want %v", disposition, Replaced)
	}
	if got := readFile(t, root, "tex/01-chapter.tex"); got != "NEW\n" {
		t.Errorf("tex = %q, want the regenerated content", got)
	}
}

func TestReconcilePreservesHandEditWhenProseChangesElsewhere(t *testing.T) {
	root, config := mergeFixture(t)
	// The author hand-edited a float near the top of the chapter.
	handEdited := strings.Replace(chapterBody, "Paragraph one.",
		"\\begin{table*}[!t]\\centering HAND EDIT \\end{table*}", 1)
	writeFile(t, root, "tex/01-chapter.tex", handEdited)
	writeFile(t, root, "tex/.baseline/01-chapter.tex", chapterBody)
	// Meanwhile the markdown changed a paragraph near the bottom.
	regenerated := strings.Replace(chapterBody, "Paragraph six.", "Paragraph six, revised.", 1)

	disposition, err := reconcile(root, config, "01-chapter.md", []byte(regenerated))
	if err != nil {
		t.Fatalf("reconcile() error: %v", err)
	}
	if disposition != Merged {
		t.Errorf("disposition = %v, want %v", disposition, Merged)
	}
	got := readFile(t, root, "tex/01-chapter.tex")
	if !strings.Contains(got, "HAND EDIT") {
		t.Errorf("the hand-edited float was lost:\n%s", got)
	}
	if !strings.Contains(got, "Paragraph six, revised.") {
		t.Errorf("the regenerated prose was not applied:\n%s", got)
	}
	if strings.Contains(got, "<<<<<<<") {
		t.Errorf("separated edits should not conflict:\n%s", got)
	}
	// The baseline advances so the next run sees no pending upstream change.
	if base := readFile(t, root, "tex/.baseline/01-chapter.tex"); base != regenerated {
		t.Error("baseline did not advance to the regenerated content")
	}
}

func TestReconcileConflictsWhenBothEditTheSameRegion(t *testing.T) {
	root, config := mergeFixture(t)
	writeFile(t, root, "tex/01-chapter.tex",
		strings.Replace(chapterBody, "Paragraph three.", "Paragraph three, by hand.", 1))
	writeFile(t, root, "tex/.baseline/01-chapter.tex", chapterBody)
	regenerated := strings.Replace(chapterBody, "Paragraph three.", "Paragraph three, from markdown.", 1)

	disposition, err := reconcile(root, config, "01-chapter.md", []byte(regenerated))
	if err != nil {
		t.Fatalf("reconcile() error: %v", err)
	}
	if disposition != Conflicted {
		t.Errorf("disposition = %v, want %v", disposition, Conflicted)
	}
	got := readFile(t, root, "tex/01-chapter.tex")
	for _, marker := range []string{"<<<<<<< hand-edited", ">>>>>>> regenerated", "by hand", "from markdown"} {
		if !strings.Contains(got, marker) {
			t.Errorf("conflicted file is missing %q:\n%s", marker, got)
		}
	}
}

// A tex file with no baseline is the one state reconcile refuses to act on:
// it cannot tell hand-edited typesetting from generated text, and guessing
// wrong destroys the author's work.
func TestReconcileRefusesTexWithNoBaseline(t *testing.T) {
	root, config := mergeFixture(t)
	writeFile(t, root, "tex/01-chapter.tex", "HAND WRITTEN, PROVENANCE UNKNOWN\n")

	disposition, err := reconcile(root, config, "01-chapter.md", []byte("GENERATED\n"))
	if err != nil {
		t.Fatalf("reconcile() error: %v", err)
	}
	if disposition != Orphaned {
		t.Errorf("disposition = %v, want %v", disposition, Orphaned)
	}
	if got := readFile(t, root, "tex/01-chapter.tex"); got != "HAND WRITTEN, PROVENANCE UNKNOWN\n" {
		t.Errorf("the orphaned file was overwritten: %q", got)
	}
	if _, err := os.Stat(baselinePath(root, config, "01-chapter.md")); err == nil {
		t.Error("a baseline was invented for an orphaned file")
	}
}

func TestGenerateTexReportsConflictsAndDispositions(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 01-clean.md\n  - 02-orphan.md\n")
	writeFile(t, root, "01-clean.md", "# Clean\n")
	writeFile(t, root, "02-orphan.md", "# Orphan\n")
	// Give the second chapter tex with no baseline.
	writeFile(t, root, "tex/02-orphan.tex", "HAND WRITTEN\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	results, err := GenerateTex(root, config)
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("GenerateTex() error = %v, want it to wrap ErrConflicts", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Disposition != Created {
		t.Errorf("01-clean disposition = %v, want %v", results[0].Disposition, Created)
	}
	if results[1].Disposition != Orphaned {
		t.Errorf("02-orphan disposition = %v, want %v", results[1].Disposition, Orphaned)
	}
	// The skeleton is still written, so a conflict does not leave the paper
	// without a main.tex.
	if _, err := os.Stat(filepath.Join(root, "tex", "main.tex")); err != nil {
		t.Errorf("main.tex was not written despite the conflict: %v", err)
	}
}

func TestGenerateTexIsStableAcrossRepeatedRuns(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n\nProse.\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := GenerateTex(root, config)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Disposition != Created {
		t.Errorf("first run disposition = %v, want %v", first[0].Disposition, Created)
	}
	before := readFile(t, root, "tex/01-first.tex")

	second, err := GenerateTex(root, config)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if second[0].Disposition != Replaced {
		t.Errorf("second run disposition = %v, want %v", second[0].Disposition, Replaced)
	}
	if after := readFile(t, root, "tex/01-first.tex"); after != before {
		t.Error("a no-change regeneration altered the tex")
	}
}

func TestMergeFileReportsConflictCount(t *testing.T) {
	base := []byte("a\nb\nc\n")
	ours := []byte("a\nOURS\nc\n")
	theirs := []byte("a\nTHEIRS\nc\n")

	merged, conflicts, err := mergeFile(ours, base, theirs)
	if err != nil {
		t.Fatalf("mergeFile() error: %v", err)
	}
	if conflicts == 0 {
		t.Error("competing edits to the same line reported no conflict")
	}
	if !strings.Contains(string(merged), "OURS") || !strings.Contains(string(merged), "THEIRS") {
		t.Errorf("conflicted output dropped a side:\n%s", merged)
	}
}

func TestDispositionStringsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for _, disposition := range []Disposition{Created, Replaced, Merged, Conflicted, Orphaned} {
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

func mergeFixture(t *testing.T) (string, Config) {
	t.Helper()
	root := paperFixture(t, "chapters:\n  - 01-chapter.md\n", "01-chapter.md")
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, config
}
