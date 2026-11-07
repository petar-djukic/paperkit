package paperkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDropEmptyBibliographyOnlyWhenNothingIsCited(t *testing.T) {
	block := "\\bibliographystyle{IEEEtran}\n\\bibliography{references}\n"
	uncited := []byte("\\section{A}\nNo citations.\n" + block + "\\end{document}\n")
	if got := string(dropEmptyBibliography(uncited)); strings.Contains(got, "\\bibliography") {
		t.Errorf("an outline citing nothing kept its bibliography:\n%s", got)
	}

	cited := []byte("\\section{A}\nSee \\cite{lee-2026}.\n" + block + "\\end{document}\n")
	if got := string(dropEmptyBibliography(cited)); got != string(cited) {
		t.Errorf("an outline with citations was altered:\n%s", got)
	}
}

func TestTemplateSearchPathListsTemplatesAndItsSubdirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, filepath.Join("templates", "preamble.tex"), "%")
	writeFile(t, root, filepath.Join("templates", "sosym", "svjour.cls"), "%")

	got, err := templateSearchPath(root)
	if err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathListSeparator)
	if !strings.HasSuffix(got, separator) {
		t.Errorf("search path %q lacks the trailing separator that keeps TeX's default path", got)
	}
	parts := strings.Split(strings.TrimSuffix(got, separator), separator)
	if len(parts) != 2 || filepath.Base(parts[0]) != "templates" || filepath.Base(parts[1]) != "sosym" {
		t.Errorf("search path = %q, want templates/ then templates/sosym", got)
	}
}

func TestOutlineRejectsAMissingOrUnknownFormat(t *testing.T) {
	root, config := buildFixture(t)
	if err := Outline(root, config); err == nil || !strings.Contains(err.Error(), "no outline format") {
		t.Errorf("Outline() with no format = %v, want it to name the missing setting", err)
	}
	config.Outline.Format = "html"
	if err := Outline(root, config); err == nil || !strings.Contains(err.Error(), "html") {
		t.Errorf("Outline() with format html = %v, want it rejected", err)
	}
}
