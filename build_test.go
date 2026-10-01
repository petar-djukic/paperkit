package paperkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckSourcesReportsMissingLatex(t *testing.T) {
	root, config := buildFixture(t)

	problems, err := checkSources(root, config)
	if err != nil {
		t.Fatalf("checkSources() error: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "has no generated LaTeX") {
		t.Errorf("problems = %v, want one missing-LaTeX report", problems)
	}
}

func TestCheckSourcesReportsStaleLatex(t *testing.T) {
	root, config := buildFixture(t)
	writeFile(t, root, "tex/01-chapter.tex", "generated\n")
	// The markdown is edited after the LaTeX was generated.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(root, "01-chapter.md"), future, future); err != nil {
		t.Fatal(err)
	}

	problems, err := checkSources(root, config)
	if err != nil {
		t.Fatalf("checkSources() error: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "is newer than its LaTeX") {
		t.Errorf("problems = %v, want one staleness report", problems)
	}
}

func TestCheckSourcesReportsMissingFigureAndAcceptsPresentOne(t *testing.T) {
	root, config := buildFixture(t)
	writeFile(t, root, "tex/01-chapter.tex",
		`\includegraphics{fig/present.pdf}`+"\n"+`\includegraphics[width=2in]{fig/absent.pdf}`+"\n")
	writeFile(t, root, filepath.Join("fig", "present.pdf"), "%PDF")

	problems, err := checkSources(root, config)
	if err != nil {
		t.Fatalf("checkSources() error: %v", err)
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want exactly one", problems)
	}
	if !strings.Contains(problems[0], "absent.pdf") {
		t.Errorf("problem %q does not name the missing figure", problems[0])
	}
}

func TestCheckSourcesPassesOnACurrentPaper(t *testing.T) {
	root, config := buildFixture(t)
	writeFile(t, root, "tex/01-chapter.tex", "generated, no figures\n")

	problems, err := checkSources(root, config)
	if err != nil {
		t.Fatalf("checkSources() error: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("problems = %v, want none", problems)
	}
}

func TestFigureExistsResolvesLikeGraphicx(t *testing.T) {
	root, config := buildFixture(t)
	writeFile(t, root, filepath.Join("fig", "chart.pdf"), "%PDF")
	writeFile(t, root, filepath.Join("images", "photo.png"), "PNG")

	for name, want := range map[string]bool{
		// Extensionless, found by trying the usual suffixes.
		"chart": true,
		// Relative to the figure directory.
		"chart.pdf": true,
		// Relative to the paper root.
		"images/photo.png": true,
		"fig/chart.pdf":    true,
		"missing.pdf":      false,
		"images/gone.png":  false,
	} {
		if got := figureExists(root, config, name); got != want {
			t.Errorf("figureExists(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestOutOfDateComparesModificationTimes(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "a.d2")
	output := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !outOfDate(source, output) {
		t.Error("a missing output should be out of date")
	}

	if err := os.WriteFile(output, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(output, future, future); err != nil {
		t.Fatal(err)
	}
	if outOfDate(source, output) {
		t.Error("an output newer than its source should be current")
	}

	if err := os.Chtimes(source, future.Add(time.Hour), future.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !outOfDate(source, output) {
		t.Error("a source newer than its output should be out of date")
	}

	if outOfDate(filepath.Join(dir, "absent.d2"), output) {
		t.Error("a missing source is nothing to rebuild from")
	}
}

func TestFiguresCompilesStaleOnly(t *testing.T) {
	if _, err := exec.LookPath("d2"); err != nil {
		t.Skip("d2 is not installed")
	}
	root, config := buildFixture(t)
	writeFile(t, root, filepath.Join("fig", "simple.d2"), "a -> b\n")

	if err := Figures(root, config); err != nil {
		t.Fatalf("Figures() error: %v", err)
	}
	output := filepath.Join(root, "fig", "simple.pdf")
	first, err := os.Stat(output)
	if err != nil {
		t.Fatalf("no figure produced: %v", err)
	}

	if err := Figures(root, config); err != nil {
		t.Fatalf("second Figures() error: %v", err)
	}
	second, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if !second.ModTime().Equal(first.ModTime()) {
		t.Error("a current figure was recompiled")
	}
}

func TestFiguresIsANoOpWithoutSources(t *testing.T) {
	root, config := buildFixture(t)
	if err := Figures(root, config); err != nil {
		t.Errorf("Figures() with no d2 sources: %v", err)
	}
}

func TestCleanRemovesArtifactsButKeepsTexSources(t *testing.T) {
	root, config := buildFixture(t)
	writeFile(t, root, "build/main.aux", "aux")
	writeFile(t, root, filepath.Join("fig", "chart.pdf"), "%PDF")
	writeFile(t, root, filepath.Join("fig", "chart.d2"), "a -> b")
	writeFile(t, root, "tex/01-chapter.tex", "hand-edited")
	writeFile(t, root, "tex/.baseline/01-chapter.tex", "baseline")

	if err := Clean(root, config); err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	for _, gone := range []string{"build", filepath.Join("fig", "chart.pdf")} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("%s survived Clean()", gone)
		}
	}
	// tex/ holds sources and their baselines, not artifacts.
	for _, kept := range []string{"tex/01-chapter.tex", "tex/.baseline/01-chapter.tex", filepath.Join("fig", "chart.d2")} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(kept))); err != nil {
			t.Errorf("Clean() removed %s", kept)
		}
	}
}

func TestDedupeKeepsFirstOccurrenceOrder(t *testing.T) {
	got := dedupe([]string{"b", "a", "b", "c", "a"})
	want := []string{"b", "a", "c"}
	if len(got) != len(want) {
		t.Fatalf("dedupe() = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("dedupe()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestLoadConfigDefaultsToIEEEtran(t *testing.T) {
	root := paperFixture(t, "chapters:\n  - 01-intro.md\n", "01-intro.md")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.DocumentClass != "IEEEtran" || config.ClassOptions != "journal" {
		t.Errorf("class = %s[%s], want IEEEtran[journal]", config.DocumentClass, config.ClassOptions)
	}
	// Both defaults stay inside the paper directory: a default that reaches
	// into a parent is exactly what keeps a copied paper from building.
	if config.Bibliography != "references.yaml" {
		t.Errorf("Bibliography = %q, want the paper's own corpus", config.Bibliography)
	}
	if config.RefsTool != "github.com/petar-djukic/paperkit/cmd/refs2bib" {
		t.Errorf("RefsTool = %q, want this module's converter", config.RefsTool)
	}
}

func buildFixture(t *testing.T) (string, Config) {
	t.Helper()
	root := paperFixture(t, "chapters:\n  - 01-chapter.md\n", "01-chapter.md")
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, config
}

func TestCheckMissingCharsPassesOnACleanLog(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.log", "This is XeTeX\nOutput written on main.pdf (22 pages).\n")

	if err := CheckMissingChars(filepath.Join(root, "main.log")); err != nil {
		t.Errorf("CheckMissingChars() = %v, want nil for a log with no warnings", err)
	}
}

func TestCheckMissingCharsFailsOnDroppedGlyphs(t *testing.T) {
	root := t.TempDir()
	// latexmk exits zero on these, so only reading the log catches them.
	writeFile(t, root, "main.log", strings.Join([]string{
		`Missing character: There is no ^^^^202f (U+202F) in font [lmroman10]!`,
		`Missing character: There is no ^^^^202f (U+202F) in font [lmroman10]!`,
		`Missing character: There is no ^^^^2011 (U+2011) in font [lmroman10]!`,
	}, "\n"))

	err := CheckMissingChars(filepath.Join(root, "main.log"))
	if err == nil {
		t.Fatal("CheckMissingChars() = nil, want an error naming the dropped glyphs")
	}
	if !strings.Contains(err.Error(), "2 character(s) missing") {
		t.Errorf("error = %q, want it to count the two distinct code points", err)
	}
}

func TestCheckMissingCharsIgnoresAnAbsentLog(t *testing.T) {
	// A build that never got as far as writing a log leaves latexmk's own exit
	// status to speak; inventing an error here would mask it.
	if err := CheckMissingChars(filepath.Join(t.TempDir(), "main.log")); err != nil {
		t.Errorf("CheckMissingChars() = %v, want nil when there is no log", err)
	}
}
