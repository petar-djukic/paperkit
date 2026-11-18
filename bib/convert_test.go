package bib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectInitialConvertsNonASCIILeadingLetters(t *testing.T) {
	for given, want := range map[string]string{
		// Decomposes to a base letter plus a combining mark this knows.
		"Éric":   `{\'E}ric`,
		"Álvaro": `{\'A}lvaro`,
		"Ötzi":   `{\"O}tzi`,
		"Çağla":  `{\c C}ağla`,
		// No combining mark to decompose; covered by the stroked-letter table.
		"Łukasz": `{\L}ukasz`,
		"Øyvind": `{\O}yvind`,
		// ASCII, already protected, or already a control sequence: untouched.
		"Andrew":   "Andrew",
		`{\'E}ric`: `{\'E}ric`,
		`\'Eric`:   `\'Eric`,
		"":         "",
		"José":     "José",
	} {
		if got := protectInitial(given); got != want {
			t.Errorf("protectInitial(%q) = %q, want %q", given, got, want)
		}
	}
}

func TestProtectAccentedInitialsRewritesWrappedAndNestedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "references.bib")
	// The two shapes that broke earlier attempts: a value wrapped across lines,
	// and a corporate author arriving as a nested brace group.
	source := `@article{a,
  author = {Rutten, Éric and Marchand, Nicolas},
  title = {One}
}
@misc{b,
  author = {{OpenAI} and Achiam, Josh and Kaiser, Łukasz and
    Kondraciuk, Łukasz},
  title = {Two}
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := protectAccentedInitials(path); err != nil {
		t.Fatalf("protectAccentedInitials() error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{`{\'E}ric`, `{\L}ukasz`} {
		if !strings.Contains(text, want) {
			t.Errorf("%s missing:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Éric") || strings.Contains(text, "Łukasz") {
		t.Errorf("a raw accented initial survived:\n%s", text)
	}
	// The corporate author is a nested group, not a "Family, Given" pair, and
	// must come through untouched.
	if !strings.Contains(text, "{OpenAI}") {
		t.Errorf("the corporate author was mangled:\n%s", text)
	}
	// ASCII names are left exactly as they were.
	for _, keep := range []string{"Marchand, Nicolas", "Achiam, Josh"} {
		if !strings.Contains(text, keep) {
			t.Errorf("%s was altered:\n%s", keep, text)
		}
	}
}

func TestProtectAccentedInitialsLeavesAnASCIIOnlyFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "references.bib")
	source := "@article{a,\n  author = {Smith, John and Doe, Jane},\n  title = {One}\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := protectAccentedInitials(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != source {
		t.Errorf("an ASCII-only file was rewritten:\n%s", got)
	}
}

func TestReportInstitutionsRenamesPublisherInTechreports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "references.bib")
	// A report pandoc wrote from CSL (publisher), one that already names its
	// institution, and a book whose publisher must stay a publisher.
	source := `@techreport{feiler-2006,
  author = {Feiler, Peter H. and Gluch, David P.},
  publisher = {Software Engineering Institute, Carnegie Mellon
    University},
  title = {The {AADL}},
  number = {CMU/SEI-2006-TN-011},
  year = {2006}
}
@techreport{both,
  institution = {Kept},
  publisher = {Also kept},
  title = {Both}
}
@book{fowler,
  author = {Fowler, Martin},
  publisher = {Addison-Wesley},
  title = {Domain-Specific Languages}
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reportInstitutions(path); err != nil {
		t.Fatalf("reportInstitutions() error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "  institution = {Software Engineering Institute, Carnegie Mellon\n    University},") {
		t.Errorf("the report's publisher was not renamed, or its wrapped value changed:\n%s", text)
	}
	if !strings.Contains(text, "  institution = {Kept},\n  publisher = {Also kept},") {
		t.Errorf("an entry that already had an institution was altered:\n%s", text)
	}
	if !strings.Contains(text, "  publisher = {Addison-Wesley},") {
		t.Errorf("a book's publisher was renamed:\n%s", text)
	}
}

func TestReportInstitutionsLeavesAFileWithoutReportsUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "references.bib")
	source := "@book{fowler,\n  publisher = {Addison-Wesley},\n  title = {DSLs}\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := reportInstitutions(path); err != nil {
		t.Fatalf("reportInstitutions() error: %v", err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("a file with no @techreport entry was rewritten")
	}
}

func TestGenericArticleVenuesNamesJournalPapersTypedAsArticle(t *testing.T) {
	entries := []map[string]interface{}{
		{"id": "journal", "type": "article", "container-title": "Science of Computer Programming"},
		{"id": "preprint", "type": "article", "container-title": "arXiv preprint arXiv:2308.00352"},
		{"id": "bare", "type": "article"},
		{"id": "typed", "type": "article-journal", "container-title": "Computer"},
	}
	got := genericArticleVenues(entries)
	if len(got) != 1 || got[0] != "journal" {
		t.Errorf("genericArticleVenues() = %v, want [journal]", got)
	}
}
