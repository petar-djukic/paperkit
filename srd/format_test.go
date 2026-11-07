package srd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fixtureSRD exercises every field kind the format defines, including the
// shapes that are easy to lose: empty lists, a table with a header and no
// rows, a table with no columns, a false wide flag, a citation with no note,
// an artifact, a narrow no-break space, and comments.
const fixtureSRD = `# SRD S2 -- Fixture (GH-1).
meta:
  unit: S2
  title: 'Fixture: every field'
  parent: null
  children: [S2.1]
section_goal: show every field in one document
goals:
  - id: G1.1
    goal: The first goal.
  - {id: G2.1, goal: "The second goal,
      folded over two lines."}
objective: |
  First paragraph of the objective.

  Second paragraph, with Section` + " " + `2.1 kept.
claims_posture: Demonstrated only.
prior_material:
  - path: ../prior/02-model.md
    offers: the old model chapter
evidence: [exp01-baseline]
# Citations carry a role.
citations:
  - {id: lee-2026, role: anchor, note: why it is cited}
  - {id: kim-2025, role: evidence}
patterns: []
content:
  - say: The first thing to say.
    carried_by: [lee-2026, kim-2025]
  - say: A planned table goes here.
    artifact: Table 2 (planned)
  - say: An unsupported claim, flagged (GH-12).
tables:
  - shows: the comparison
    columns: [Name, Value | unit]
    rows:
      - [alpha, '1']
      - [beta, 'a | b']
    status: ready
    wide: false
  - shows: a header and no rows
    columns: [A, B]
    status: planned
  - shows: no columns yet
    wide: true
figures:
  - {file: d2/overview.d2, status: ready, shows: the whole system, wide: true}
  - {file: fig/levels.pdf, status: planned, shows: the levels}
links:
  supports: [S3, S4]
  requires: []
acceptance:
  - The section states the goal.
gaps: []
`

const fixtureVision = `meta:
  name: fixture
  started: 2026-08-01
thesis: One sentence of thesis.
goals:
  - {id: G1, goal: The reader can build it.}
non_goals: [Not a survey.]
inputs:
  standards: TM Forum and ETSI
`

func TestToMarkdownRoundTripsEveryFieldKind(t *testing.T) {
	for name, fixture := range map[string]struct {
		source string
		kind   DocKind
	}{
		"srd":    {fixtureSRD, SRDDoc},
		"vision": {fixtureVision, VisionDoc},
	} {
		t.Run(name, func(t *testing.T) {
			markdown, err := ToMarkdown([]byte(fixture.source), fixture.kind)
			if err != nil {
				t.Fatalf("ToMarkdown() error: %v", err)
			}
			var original map[string]interface{}
			if err := yaml.Unmarshal([]byte(fixture.source), &original); err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseMarkdown(markdown, fixture.kind)
			if err != nil {
				t.Fatalf("ParseMarkdown() error: %v\n%s", err, markdown)
			}
			if !reflect.DeepEqual(Normalize(original), Normalize(parsed)) {
				t.Errorf("round trip changed the document\nyaml: %#v\n  md: %#v\n%s", Normalize(original), Normalize(parsed), markdown)
			}
		})
	}
}

func TestToMarkdownLocksContractFieldsAndKeepsComments(t *testing.T) {
	markdown, err := ToMarkdown([]byte(fixtureSRD), SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	text := string(markdown)
	for _, field := range []string{"section_goal", "goals", "acceptance"} {
		section := text[strings.Index(text, "<!-- field: "+field+" -->"):]
		next := strings.Index(section[1:], "<!-- field: ")
		if next > 0 {
			section = section[:next+1]
		}
		if !strings.Contains(section, "<!-- lock -->") || !strings.Contains(section, "<!-- /lock -->") {
			t.Errorf("%s is not inside a block lock:\n%s", field, section)
		}
	}
	for _, want := range []string{"SRD S2 -- Fixture (GH-1).", "Citations carry a role."} {
		if !strings.Contains(text, want) {
			t.Errorf("comment %q was dropped", want)
		}
	}
	// Says are paragraphs, so prose tooling sees them; citations ride inline.
	if !strings.Contains(text, "\nThe first thing to say. [@lee-2026; @kim-2025]\n") {
		t.Errorf("the first say is not a paragraph with its citations:\n%s", text)
	}
	if !strings.Contains(text, "Section 2.1") {
		t.Error("the narrow no-break space did not survive")
	}
}

func TestParseMarkdownCollectsMovedCitations(t *testing.T) {
	markdown, err := ToMarkdown([]byte(fixtureSRD), SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	// A polish pass that moves the citation group into the sentence must not
	// lose the ids or leave them in the say.
	moved := strings.Replace(string(markdown), "The first thing to say. [@lee-2026; @kim-2025]",
		"The first [@lee-2026; @kim-2025] thing to say.", 1)
	parsed, err := ParseMarkdown([]byte(moved), SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	first := parsed["content"].([]interface{})[0].(map[string]interface{})
	if first["say"] != "The first thing to say." || !reflect.DeepEqual(first["carried_by"], []interface{}{"lee-2026", "kim-2025"}) {
		t.Errorf("moved citation read as %#v", first)
	}
}

func TestParseMarkdownDetectsAnEditedValue(t *testing.T) {
	markdown, err := ToMarkdown([]byte(fixtureSRD), SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]interface{}
	yaml.Unmarshal([]byte(fixtureSRD), &original)
	edited := bytes.Replace(markdown, []byte("| beta | a \\| b |"), []byte("| beta | a b |"), 1)
	parsed, err := ParseMarkdown(edited, SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(Normalize(original), Normalize(parsed)) {
		t.Error("an edited table cell still compared equal")
	}
}

func TestFormatRejectsWhatItCannotCarry(t *testing.T) {
	for name, source := range map[string]string{
		"unknown field":        "meta: {unit: S1}\nsynopsis: not in the format\n",
		"unknown record key":   "meta: {unit: S1}\ncontent:\n  - {say: x, weight: 3}\n",
		"say with a citation":  "meta: {unit: S1}\ncontent:\n  - {say: 'see [@lee-2026]'}\n",
		"say that is a bullet": "meta: {unit: S1}\ncontent:\n  - {say: '- a list item'}\n",
		"empty carried_by":     "meta: {unit: S1}\ncontent:\n  - {say: x, carried_by: []}\n",
		"no meta":              "section_goal: x\n",
	} {
		if _, err := ToMarkdown([]byte(source), SRDDoc); err == nil {
			t.Errorf("%s: ToMarkdown() accepted it", name)
		}
	}
	if _, err := ParseMarkdown([]byte("---\nunit: S1\n---\n<!-- field: synopsis -->\n"), SRDDoc); err == nil {
		t.Error("ParseMarkdown() accepted an unknown field")
	}
	if _, err := ParseMarkdown([]byte("---\nunit: S1\n---\nstray text\n"), SRDDoc); err == nil {
		t.Error("ParseMarkdown() accepted text outside any field")
	}
}

// writeDocs lays out a docs directory with the fixtures in the given form.
func writeDocs(t *testing.T, form string) string {
	t.Helper()
	paper := t.TempDir()
	docs := filepath.Join(paper, "docs")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(paper, "00-title-page.md"), "---\ntitle: Fixture Paper\nabstract: An abstract.\n---\n")
	for name, source := range map[string]string{"srd/srd-2-fixture": fixtureSRD, "VISION": fixtureVision} {
		if form == "yaml" {
			write(filepath.Join(docs, name+".yaml"), source)
			continue
		}
		markdown, err := ToMarkdown([]byte(source), KindOf(name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(docs, name+".md"), string(markdown))
	}
	return docs
}

func TestRenderIsIdenticalFromEitherForm(t *testing.T) {
	yamlDocs, mdDocs := writeDocs(t, "yaml"), writeDocs(t, "md")
	for _, format := range []Format{TeX, Markdown} {
		fromYAML, err := Render(yamlDocs, format)
		if err != nil {
			t.Fatalf("Render(yaml, %s): %v", format, err)
		}
		fromMarkdown, err := Render(mdDocs, format)
		if err != nil {
			t.Fatalf("Render(md, %s): %v", format, err)
		}
		// The paper directory's name reaches the tex outline's byline.
		fromMarkdown = bytes.ReplaceAll(fromMarkdown, []byte(filepath.Base(filepath.Dir(mdDocs))), []byte(filepath.Base(filepath.Dir(yamlDocs))))
		if !bytes.Equal(fromYAML, fromMarkdown) {
			t.Errorf("%s outline differs between the forms\nyaml:\n%s\nmd:\n%s", format, fromYAML, fromMarkdown)
		}
	}
}

func TestPathsRefuseADocumentInBothForms(t *testing.T) {
	docs := writeDocs(t, "md")
	if err := os.WriteFile(filepath.Join(docs, "srd", "srd-2-fixture.yaml"), []byte(fixtureSRD), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SRDPaths(docs); err == nil {
		t.Error("SRDPaths() accepted an SRD present as both .yaml and .md")
	}
	if err := os.WriteFile(filepath.Join(docs, "VISION.yaml"), []byte(fixtureVision), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VisionPath(docs); err == nil {
		t.Error("VisionPath() accepted VISION as both .yaml and .md")
	}
}
