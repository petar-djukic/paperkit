package paperkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTexWritesFragmentPerChapterAndSkeleton(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 01-first.md\n  - 02-second.md\n")
	writeFile(t, root, "01-first.md", "# First\n\nProse citing [@lee-2026].\n")
	writeFile(t, root, "02-second.md", "# Second\n\nA list:\n\n- one\n- two\n")

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
	if !strings.Contains(first, `\citep{lee-2026}`) {
		t.Errorf("01-first.tex did not render the citation:\n%s", first)
	}
	// A fragment must not carry its own preamble, or \input would nest a
	// second document inside the skeleton.
	if strings.Contains(first, `\documentclass`) || strings.Contains(first, `\begin{document}`) {
		t.Errorf("01-first.tex is standalone, want a fragment:\n%s", first)
	}
	if second := readFile(t, root, "tex/02-second.tex"); !strings.Contains(second, `\begin{itemize}`) {
		t.Errorf("02-second.tex did not render the list:\n%s", second)
	}
}

func TestGenerateTexSkeletonInputsChaptersInConfigOrder(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 02-second.md\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n")
	writeFile(t, root, "02-second.md", "# Second\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}

	main := readFile(t, root, "tex/main.tex")
	second := strings.Index(main, `\input{02-second}`)
	first := strings.Index(main, `\input{01-first}`)
	if second < 0 || first < 0 {
		t.Fatalf("main.tex is missing chapter inputs:\n%s", main)
	}
	if second > first {
		t.Errorf("main.tex inputs chapters in filename order, want config order:\n%s", main)
	}
	for _, want := range []string{`\documentclass`, `\begin{document}`, `\bibliography{references}`, `\end{document}`} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tex is missing %s", want)
		}
	}
}

func TestGenerateTexRewritesBibliographyStyle(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}

	main := readFile(t, root, "tex/main.tex")
	if !strings.Contains(main, `\bibliographystyle{IEEEtranN}`) {
		t.Errorf("main.tex does not use IEEEtranN:\n%s", main)
	}
	if strings.Contains(main, `plainnat`) {
		t.Error("main.tex still carries pandoc's plainnat default")
	}
}

func TestGenerateTexIncludesVenuePreamble(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, "chapters:\n  - 01-first.md\n")
	writeFile(t, root, "01-first.md", "# First\n")
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"),
		"% preamble\n\\usepackage{cuted}\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateTex(root, config); err != nil {
		t.Fatal(err)
	}
	if main := readFile(t, root, "tex/main.tex"); !strings.Contains(main, `\usepackage{cuted}`) {
		t.Errorf("main.tex did not include the venue preamble:\n%s", main)
	}
}

// A magefile passes "." as the paper root, so every path the package hands to
// an external command has to be correct relative to that. Paths joined with
// root and then run with root as the working directory resolve inside
// themselves, which absolute-root tests cannot catch.
func TestGenerateTexAcceptsRelativeRoot(t *testing.T) {
	requirePandoc(t)
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	writeFile(t, root, "paper.yaml", "chapters:\n  - 01-first.md\n")
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"), "% preamble\n")
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

func TestSpliceBodyReportsUnrecognizedSkeleton(t *testing.T) {
	for name, skeleton := range map[string]string{
		"no begin":        `\documentclass{article}` + "\n" + `\bibliography{references}`,
		"no bibliography": `\documentclass{article}` + "\n" + `\begin{document}` + "\n" + `\end{document}`,
	} {
		if _, err := spliceBody(skeleton, `\input{01-first}`); err == nil {
			t.Errorf("spliceBody(%s) unexpectedly passed", name)
		}
	}
}

func TestSpliceBodyKeepsPreambleAndEndMatter(t *testing.T) {
	skeleton := "PREAMBLE\n" + beginDocument + "\n\n\n" + bibliographyLine + "\n\n" + `\end{document}` + "\n"

	got, err := spliceBody(skeleton, `\input{01-first}`+"\n")
	if err != nil {
		t.Fatalf("spliceBody() error: %v", err)
	}
	if !strings.HasPrefix(got, "PREAMBLE\n"+beginDocument) {
		t.Errorf("preamble was not preserved:\n%s", got)
	}
	if !strings.Contains(got, `\input{01-first}`) {
		t.Errorf("body was not spliced in:\n%s", got)
	}
	if !strings.Contains(got, bibliographyLine+"\n\n"+`\end{document}`) {
		t.Errorf("end matter was not preserved:\n%s", got)
	}
}

func requirePandoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc is not installed")
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNamespaceLabelsPrefixesOnlyDerivedLabels(t *testing.T) {
	source := []byte("# Agent View {#chapter-agent-view}\n\n## Introduction\n\n" +
		"Table: Roles {#tab-agent-roles}\n")
	generated := []byte(`\section{Agent View}\label{chapter-agent-view}` + "\n" +
		`\subsection{Introduction}\label{introduction}` + "\n" +
		`\label{tab-agent-roles}` + "\n" +
		`\hyperref[fig-whole-system]{the figure}` + "\n" +
		`\label{fig-whole-system}` + "\n")

	got := string(namespaceLabels(generated, source, "02-agent-view.md"))

	// Derived from heading text, and nothing points at it: namespaced.
	if !strings.Contains(got, `\label{02-agent-view:introduction}`) {
		t.Errorf("the derived label was not namespaced:\n%s", got)
	}
	// Authored in the markdown: left alone, or every cross-chapter link breaks.
	for _, keep := range []string{`\label{chapter-agent-view}`, `\label{tab-agent-roles}`} {
		if !strings.Contains(got, keep) {
			t.Errorf("%s was namespaced but is authored:\n%s", keep, got)
		}
	}
	// Minted by a filter rather than the author, but referenced here, so
	// namespacing it would dangle the reference.
	if !strings.Contains(got, `\label{fig-whole-system}`) {
		t.Errorf("a referenced label was namespaced:\n%s", got)
	}
}

func TestNamespaceLabelsIsIdempotent(t *testing.T) {
	source := []byte("## Introduction\n")
	generated := []byte(`\label{introduction}`)

	once := namespaceLabels(generated, source, "05-information-view.md")
	twice := namespaceLabels(once, source, "05-information-view.md")
	if string(once) != string(twice) {
		t.Errorf("second pass changed the labels:\n%s\n%s", once, twice)
	}
	if !strings.Contains(string(once), `\label{05-information-view:introduction}`) {
		t.Errorf("unexpected label: %s", once)
	}
}

// The collision this exists to prevent: two chapters that both open with an
// "Introduction" heading produce the same label when converted separately.
func TestNamespaceLabelsSeparatesIdenticalHeadingsAcrossChapters(t *testing.T) {
	source := []byte("## Introduction\n")
	generated := []byte(`\label{introduction}`)

	a := string(namespaceLabels(generated, source, "02-agent-view.md"))
	b := string(namespaceLabels(generated, source, "03-user-view.md"))
	if a == b {
		t.Errorf("both chapters produced %s", a)
	}
}
