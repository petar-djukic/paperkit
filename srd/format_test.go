package srd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fixtureSRD is the document testdata/docs/srd/srd-2-fixture.md holds, in the
// YAML form it was converted from. It exercises every field kind, including
// the shapes that are easy to lose: empty lists, a table with a header and no
// rows, a table with no columns, a false wide flag, a citation with no note,
// an artifact, a narrow no-break space, and comments. The markdown fixture
// was written by the converter (paperkit v0.20261107.5) and verified against
// it; the test holds it to the same content.
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

// fixtureVision is the document testdata/docs/VISION.md holds.
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

// normalize collapses every string's ASCII whitespace, the equality the
// markdown form promises: it keeps text, not line layout.
func normalize(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		return collapse(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for index, item := range t {
			out[index] = normalize(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for key, item := range t {
			out[key] = normalize(item)
		}
		return out
	}
	return v
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFixturesParseToTheirDocuments(t *testing.T) {
	for name, expected := range map[string]string{
		"srd/srd-2-fixture.md": fixtureSRD,
		"VISION.md":            fixtureVision,
	} {
		t.Run(name, func(t *testing.T) {
			var want map[string]interface{}
			if err := yaml.Unmarshal([]byte(expected), &want); err != nil {
				t.Fatal(err)
			}
			got, err := Load(filepath.Join("testdata", "docs", name))
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if !reflect.DeepEqual(normalize(want), normalize(got)) {
				t.Errorf("%s parsed differently\nwant: %#v\n got: %#v", name, normalize(want), normalize(got))
			}
		})
	}
}

func TestFixtureLocksContractFieldsAndCarriesCitationsInline(t *testing.T) {
	text := fixture(t, "srd/srd-2-fixture.md")
	for _, field := range []string{"section_goal", "goals", "acceptance"} {
		section := text[strings.Index(text, "<!-- field: "+field+" -->"):]
		if next := strings.Index(section[1:], "<!-- field: "); next > 0 {
			section = section[:next+1]
		}
		if !strings.Contains(section, "<!-- lock -->") || !strings.Contains(section, "<!-- /lock -->") {
			t.Errorf("%s is not inside a block lock:\n%s", field, section)
		}
	}
	if !strings.Contains(text, "\nThe first thing to say. [@lee-2026; @kim-2025]\n") {
		t.Errorf("the first say is not a paragraph with its citations:\n%s", text)
	}
	if !strings.Contains(text, "Section\u202f2.1") {
		t.Error("the narrow no-break space is missing from the fixture")
	}
}

// A polish pass that moves the citation group into the sentence must not
// lose the ids or leave them in the say.
func TestParseMarkdownCollectsMovedCitations(t *testing.T) {
	moved := strings.Replace(fixture(t, "srd/srd-2-fixture.md"), "The first thing to say. [@lee-2026; @kim-2025]",
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
	var want map[string]interface{}
	if err := yaml.Unmarshal([]byte(fixtureSRD), &want); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(fixture(t, "srd/srd-2-fixture.md"), "| beta | a \\| b |", "| beta | a b |", 1)
	parsed, err := ParseMarkdown([]byte(edited), SRDDoc)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(normalize(want), normalize(parsed)) {
		t.Error("an edited table cell still compared equal")
	}
}

func TestParseMarkdownRejectsWhatTheFormatDoesNotDefine(t *testing.T) {
	for name, text := range map[string]string{
		"unknown field":        "---\nunit: S1\n---\n<!-- field: synopsis -->\n",
		"text outside":         "---\nunit: S1\n---\nstray text\n",
		"field twice":          "---\nunit: S1\n---\n<!-- field: objective -->\nOne.\n<!-- field: objective -->\nTwo.\n",
		"no front matter":      "<!-- field: objective -->\nOne.\n",
		"artifact with no say": "---\nunit: S1\n---\n<!-- field: content -->\n<!-- artifact: Table 1 -->\n",
	} {
		if _, err := ParseMarkdown([]byte(text), SRDDoc); err == nil {
			t.Errorf("%s: ParseMarkdown() accepted it", name)
		}
	}
}

func TestRenderMatchesTheGoldenOutlines(t *testing.T) {
	for _, format := range []Format{TeX, Markdown} {
		got, err := Render(filepath.Join("testdata", "docs"), format)
		if err != nil {
			t.Fatalf("Render(%s): %v", format, err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "outline."+string(format)+".golden"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s outline differs from the golden\n got:\n%s\nwant:\n%s", format, got, want)
		}
	}
}

// The YAML form is retired: a stale file is an error, never skipped.
func TestRetiredYAMLIsAnError(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(filepath.Join(docs, "srd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "srd", "srd-9-stale.yaml"), []byte(fixtureSRD), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SRDPaths(docs); err == nil || !strings.Contains(err.Error(), "srd-9-stale.yaml") {
		t.Errorf("SRDPaths() = %v, want it to name the YAML SRD", err)
	}
	if err := os.WriteFile(filepath.Join(docs, "VISION.yaml"), []byte(fixtureVision), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VisionPath(docs); err == nil {
		t.Error("VisionPath() accepted a VISION.yaml")
	}
	if _, err := Load(filepath.Join(docs, "VISION.yaml")); err == nil {
		t.Error("Load() read a YAML document")
	}
}
