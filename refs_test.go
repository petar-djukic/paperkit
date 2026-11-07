package paperkit

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const corpusFixture = `- id: shared-fixed
  type: article-journal
  title: Corrected Title
  author:
    - family: Huang
      given: Xuanjing
- id: shared-same
  type: book
  title: Same Everywhere
- id: corpus-only
  type: book
  title: Not Cited Here
`

func refsFixture(t *testing.T, paper string) (string, Config) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	writeFile(t, parent, "references.yaml", corpusFixture)
	writeFile(t, root, "paper.yaml", "corpus: ../references.yaml\nchapters:\n  - 01-intro.md\n")
	writeFile(t, root, "01-intro.md", "# Intro\n")
	writeFile(t, root, filepath.Join("templates", "ieee-preamble.tex"), "% preamble\n")
	writeFile(t, root, "references.yaml", paper)
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, config
}

// Entries equal in content but quoted differently are not drift; entries the
// corpus does not hold are the paper's own and never drift.
func TestReferenceDriftNamesDifferingEntries(t *testing.T) {
	root, config := refsFixture(t, "- id: shared-fixed\n  title: Stale Title\n- id: shared-same\n  type: book\n  title: 'Same Everywhere'\n- id: local-only\n  title: Only Here\n")

	drift, err := ReferenceDrift(root, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drift, []string{"shared-fixed"}) {
		t.Errorf("drift = %v, want [shared-fixed]", drift)
	}
}

func TestReferenceDriftIsSilentWithoutACorpus(t *testing.T) {
	root, config := refsFixture(t, "- id: shared-fixed\n  title: Stale Title\n")
	// A copied-out paper: paper.yaml still names the corpus, but it is gone.
	if err := os.Remove(filepath.Join(filepath.Dir(root), "references.yaml")); err != nil {
		t.Fatal(err)
	}
	drift, err := ReferenceDrift(root, config)
	if err != nil || drift != nil {
		t.Errorf("ReferenceDrift() = %v, %v; want nothing when the corpus is absent", drift, err)
	}

	config.Corpus = ""
	if drift, err := ReferenceDrift(root, config); err != nil || drift != nil {
		t.Errorf("ReferenceDrift() = %v, %v; want nothing when no corpus is named", drift, err)
	}
}
