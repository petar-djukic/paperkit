package mdtotex_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/petar-djukic/paperkit/mdtotex"
)

// example is one markdown and LaTeX pair a requirement document states, with
// the requirement group it belongs to.
type example struct {
	SRD      string `yaml:"srd"`
	Group    string `yaml:"group"`
	Markdown string `yaml:"markdown"`
	LaTeX    string `yaml:"latex"`
	Note     string `yaml:"note"`
}

// srdExamples reads the worked examples md-to-tex's requirement documents
// stated. Those documents stayed with the archived md-to-tex repository; the
// pairs they carried live in testdata/examples.yaml, so the converter is still
// held to them byte for byte.
func srdExamples(t *testing.T) []example {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "examples.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var found []example
	if err := yaml.Unmarshal(data, &found); err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("testdata/examples.yaml holds no examples")
	}
	return found
}

// chapterExamples are the SRD examples whose markdown is a chapter fragment
// the conversion path takes. The others state a title page, a roster, or a
// permitted LaTeX form, and are exercised by the tests that own them.
func chapterExamples(t *testing.T) []example {
	var chapters []example
	for _, stated := range srdExamples(t) {
		switch {
		case stated.SRD == "srd001-front-matter",
			stated.SRD == "srd008-container",
			stated.SRD == "srd009-backport-compatibility":
			continue
		case strings.HasPrefix(strings.TrimSpace(stated.Markdown), "---"):
			continue
		}
		chapters = append(chapters, stated)
	}
	return chapters
}

// TestEverySRDExampleConverts covers the corpus end to end: every worked
// example a requirement document states converts to the LaTeX that document
// states, byte for byte.
//
// The unit tests assert the same pairs, but each states its expectation in Go;
// here the expectations are the requirement documents' own examples.
func TestEverySRDExampleConverts(t *testing.T) {
	examples := chapterExamples(t)
	if len(examples) < 8 {
		t.Fatalf("found %d chapter examples; the corpus states more than that", len(examples))
	}

	for _, stated := range examples {
		t.Run(stated.SRD+" "+stated.Group, func(t *testing.T) {
			result, err := mdtotex.Convert([]byte(stated.Markdown), stated.SRD+".md", mdtotex.Options{})
			if err != nil {
				t.Fatalf("Convert() error: %v\nmarkdown:\n%s", err, stated.Markdown)
			}
			if got := string(result.LaTeX); got != stated.LaTeX {
				t.Errorf("the example does not convert as the SRD states it\n got:\n%s\nwant:\n%s", got, stated.LaTeX)
			}
		})
	}
}

// TestTheFrontMatterExampleConverts covers the title-page example, which takes
// its own entry point rather than the chapter path.
func TestTheFrontMatterExampleConverts(t *testing.T) {
	for _, stated := range srdExamples(t) {
		if stated.SRD != "srd001-front-matter" {
			continue
		}
		t.Run(stated.Group, func(t *testing.T) {
			result, err := mdtotex.RenderFrontMatter([]byte(stated.Markdown), "00-front-matter.md", mdtotex.Options{})
			if err != nil {
				t.Fatalf("RenderFrontMatter() error: %v", err)
			}
			if got := string(result.LaTeX); got != stated.LaTeX {
				t.Errorf("the example does not render as the SRD states it\n got:\n%s\nwant:\n%s", got, stated.LaTeX)
			}
		})
	}
}
