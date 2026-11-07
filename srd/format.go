package srd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The markdown form of an SRD or VISION document is defined in FORMAT.md.
// The reader returns the decoded document as a map, the shape the YAML form
// the papers were first written in decoded to, so everything downstream of
// Load reads plain maps.

// DocKind names the two document types the format covers.
type DocKind int

const (
	// SRDDoc is a section requirements document, docs/srd/srd-*.
	SRDDoc DocKind = iota
	// VisionDoc is a paper's docs/VISION.
	VisionDoc
)

// KindOf infers the document type from the file name.
func KindOf(path string) DocKind {
	if strings.HasPrefix(filepath.Base(path), "VISION.") {
		return VisionDoc
	}
	return SRDDoc
}

type fieldKind int

const (
	proseField     fieldKind = iota // paragraphs of text
	listField                       // bullets, one string each
	goalsField                      // - **id** goal
	contentField                    // one paragraph per say, citations inline
	citationsField                  // - **id** (role): note
	priorField                      // - `path`: offers
	linksField                      // - name: id, id
	inputsField                     // - name: text
	tablesField                     // <!-- table | ... --> blocks
	figuresField                    // - `file` (attributes): shows
)

type fieldSpec struct {
	name    string
	heading string
	kind    fieldKind
	// locked fields are contract values, written inside a block lock so
	// prose tooling, which treats a lock region as opaque, never rewrites
	// them.
	locked bool
}

var srdFields = []fieldSpec{
	{"section_goal", "Section goal", proseField, true},
	{"goals", "Goals", goalsField, true},
	{"objective", "Objective", proseField, false},
	{"claims_posture", "Claims posture", proseField, false},
	{"prior_material", "Prior material", priorField, false},
	{"evidence", "Evidence", listField, false},
	{"citations", "Citations", citationsField, false},
	{"patterns", "Patterns", listField, false},
	{"content", "Content", contentField, false},
	{"tables", "Tables", tablesField, false},
	{"figures", "Figures", figuresField, false},
	{"links", "Links", linksField, false},
	{"acceptance", "Acceptance", listField, true},
	{"gaps", "Gaps", listField, false},
}

var visionFields = []fieldSpec{
	{"thesis", "Thesis", proseField, false},
	{"purpose", "Purpose", proseField, false},
	{"audience", "Audience", proseField, false},
	{"abstract", "Abstract", proseField, false},
	{"direction", "Direction", proseField, false},
	{"paper_model", "Paper model", proseField, false},
	{"software", "Software", proseField, false},
	{"claims_posture", "Claims posture", proseField, false},
	{"goals", "Goals", goalsField, true},
	{"non_goals", "Non-goals", listField, false},
	{"success_criteria", "Success criteria", listField, false},
	{"inputs", "Inputs", inputsField, false},
}

func fieldsOf(kind DocKind) []fieldSpec {
	if kind == VisionDoc {
		return visionFields
	}
	return srdFields
}

func specFor(kind DocKind, name string) (fieldSpec, bool) {
	for _, spec := range fieldsOf(kind) {
		if spec.name == name {
			return spec, true
		}
	}
	return fieldSpec{}, false
}

// Load reads an SRD or VISION document in the markdown form.
func Load(path string) (map[string]interface{}, error) {
	if !strings.HasSuffix(path, ".md") {
		return nil, fmt.Errorf("%s: SRDs and VISION are markdown (srd/FORMAT.md)", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := ParseMarkdown(data, KindOf(path))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// ---------------------------------------------------------------- reading

var (
	fieldTag     = regexp.MustCompile(`^<!-- field: ([a-z_]+) -->$`)
	artifactTag  = regexp.MustCompile(`^<!-- artifact: (.*) -->$`)
	citeGroupRe  = regexp.MustCompile(`\s*\[@[^\]]*\]`)
	tableTag     = regexp.MustCompile(`^<!-- table(?: \| (.*))? -->$`)
	goalLine     = regexp.MustCompile(`^- \*\*([^*]+)\*\* ?(.*)$`)
	citationLine = regexp.MustCompile(`^- \*\*([^*]+)\*\*(?: \(([^)]*)\))?(:(?: (.*))?)?$`)
	priorLine    = regexp.MustCompile("^- `([^`]*)`: ?(.*)$")
	figureLine   = regexp.MustCompile("^- `([^`]*)`(?: \\(([^)]*)\\))?: ?(.*)$")
	namedLine    = regexp.MustCompile(`^- ([a-z_]+):(?: (.*))?$`)
	citeID       = regexp.MustCompile(`@([^;\]\s]+)`)
)

// ParseMarkdown reads the markdown form back into the map the YAML decoder
// would have produced for the same document.
func ParseMarkdown(data []byte, kind DocKind) (map[string]interface{}, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("no front matter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("unterminated front matter")
	}
	var meta map[string]interface{}
	if err := yaml.Unmarshal([]byte(text[4:4+end+1]), &meta); err != nil {
		return nil, fmt.Errorf("front matter: %w", err)
	}
	doc := map[string]interface{}{"meta": meta}

	var current *fieldSpec
	var body []string
	flush := func() error {
		if current == nil {
			return nil
		}
		value, err := parseField(*current, body)
		if err != nil {
			return fmt.Errorf("%s: %w", current.name, err)
		}
		doc[current.name] = value
		return nil
	}
	inComment := false
	for _, line := range strings.Split(text[4+end+5:], "\n") {
		trimmed := strings.TrimSpace(line)
		if inComment {
			inComment = !strings.Contains(trimmed, "-->")
			continue
		}
		if match := fieldTag.FindStringSubmatch(trimmed); match != nil {
			if err := flush(); err != nil {
				return nil, err
			}
			spec, ok := specFor(kind, match[1])
			if !ok {
				return nil, fmt.Errorf("field %q is not in the format (FORMAT.md)", match[1])
			}
			if _, seen := doc[spec.name]; seen {
				return nil, fmt.Errorf("field %q appears twice", spec.name)
			}
			current, body = &spec, nil
			continue
		}
		// Comments, lock markers and the headings are for people and tools;
		// a table tag is the one comment that carries data.
		if tableTag.MatchString(trimmed) || artifactTag.MatchString(trimmed) {
			body = append(body, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			inComment = !strings.Contains(trimmed, "-->")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			continue
		}
		if current == nil {
			if trimmed != "" {
				return nil, fmt.Errorf("text outside any field: %q", trimmed)
			}
			continue
		}
		body = append(body, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return doc, nil
}

func parseField(spec fieldSpec, lines []string) (interface{}, error) {
	switch spec.kind {
	case proseField:
		var paragraphs []string
		var current []string
		for _, line := range append(lines, "") {
			if strings.TrimSpace(line) == "" {
				if len(current) > 0 {
					paragraphs = append(paragraphs, strings.Join(current, " "))
					current = nil
				}
				continue
			}
			current = append(current, strings.TrimSpace(line))
		}
		return strings.Join(paragraphs, "\n\n"), nil
	case listField:
		items := []interface{}{}
		for _, line := range nonBlank(lines) {
			if !strings.HasPrefix(line, "- ") {
				return nil, fmt.Errorf("want a bullet, have %q", line)
			}
			items = append(items, strings.TrimPrefix(line, "- "))
		}
		return items, nil
	case goalsField:
		items := []interface{}{}
		for _, line := range nonBlank(lines) {
			match := goalLine.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("want - **id** goal, have %q", line)
			}
			items = append(items, map[string]interface{}{"id": match[1], "goal": match[2]})
		}
		return items, nil
	case citationsField:
		items := []interface{}{}
		for _, line := range nonBlank(lines) {
			match := citationLine.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("want - **id** (role): note, have %q", line)
			}
			record := map[string]interface{}{"id": match[1]}
			if strings.Contains(line, "** (") {
				record["role"] = match[2]
			}
			if match[3] != "" {
				record["note"] = match[4]
			}
			items = append(items, record)
		}
		return items, nil
	case priorField:
		items := []interface{}{}
		for _, line := range nonBlank(lines) {
			match := priorLine.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("want - `path`: offers, have %q", line)
			}
			items = append(items, map[string]interface{}{"path": match[1], "offers": match[2]})
		}
		return items, nil
	case contentField:
		items := []interface{}{}
		for _, block := range blocks(lines) {
			var text []string
			record := map[string]interface{}{}
			for _, line := range block {
				if match := artifactTag.FindStringSubmatch(line); match != nil {
					record["artifact"] = match[1]
					continue
				}
				text = append(text, line)
			}
			say := strings.Join(text, " ")
			ids := []interface{}{}
			for _, group := range citeGroupRe.FindAllString(say, -1) {
				for _, match := range citeID.FindAllStringSubmatch(group, -1) {
					ids = append(ids, match[1])
				}
			}
			if len(ids) > 0 {
				record["carried_by"] = ids
			}
			record["say"] = collapse(citeGroupRe.ReplaceAllString(say, ""))
			if record["say"] == "" {
				return nil, fmt.Errorf("a content paragraph with no say")
			}
			items = append(items, record)
		}
		return items, nil
	case linksField, inputsField:
		fields := map[string]interface{}{}
		for _, line := range nonBlank(lines) {
			match := namedLine.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("want - name: value, have %q", line)
			}
			if spec.kind == inputsField {
				fields[match[1]] = match[2]
				continue
			}
			ids := []interface{}{}
			for _, id := range strings.Split(match[2], ",") {
				if id = strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
			fields[match[1]] = ids
		}
		return fields, nil
	case tablesField:
		return parseTables(nonBlank(lines))
	case figuresField:
		items := []interface{}{}
		for _, line := range nonBlank(lines) {
			match := figureLine.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("want - `file` (attributes): shows, have %q", line)
			}
			record := map[string]interface{}{"file": match[1], "shows": match[3]}
			if err := parseAttributes(record, match[2], ", "); err != nil {
				return nil, err
			}
			items = append(items, record)
		}
		return items, nil
	}
	return nil, fmt.Errorf("unknown field kind")
}

func parseTables(lines []string) (interface{}, error) {
	items := []interface{}{}
	var record map[string]interface{}
	for _, line := range lines {
		if match := tableTag.FindStringSubmatch(line); match != nil {
			record = map[string]interface{}{}
			if err := parseAttributes(record, match[1], " | "); err != nil {
				return nil, err
			}
			items = append(items, record)
			continue
		}
		if record == nil {
			return nil, fmt.Errorf("table content before a table tag: %q", line)
		}
		switch {
		case strings.HasPrefix(line, "Table: "):
			record["shows"] = strings.TrimPrefix(line, "Table: ")
		case strings.HasPrefix(line, "|---"):
			if _, ok := record["rows"]; !ok {
				record["rows"] = []interface{}{}
			}
		case strings.HasPrefix(line, "|"):
			cells := splitRow(line)
			if _, ok := record["columns"]; !ok {
				record["columns"] = cells
				continue
			}
			rows, _ := record["rows"].([]interface{})
			record["rows"] = append(rows, cells)
		default:
			return nil, fmt.Errorf("want a table line, have %q", line)
		}
	}
	// A header with no body rows was written from a table whose rows key was
	// absent; an explicitly empty rows list carries its own attribute.
	for _, item := range items {
		record := item.(map[string]interface{})
		if rows, ok := record["rows"].([]interface{}); ok && len(rows) == 0 && record["rows:[]"] == nil {
			delete(record, "rows")
		}
		delete(record, "rows:[]")
		delete(record, "columns:[]")
	}
	return items, nil
}

func parseAttributes(record map[string]interface{}, attrs, separator string) error {
	if strings.TrimSpace(attrs) == "" {
		return nil
	}
	for _, attr := range strings.Split(attrs, separator) {
		key, value, ok := strings.Cut(attr, ": ")
		if !ok {
			return fmt.Errorf("attribute %q is not key: value", attr)
		}
		switch {
		case key == "wide":
			record[key] = value == "true"
		case value == "[]":
			record[key] = []interface{}{}
			record[key+":[]"] = true
		default:
			record[key] = value
		}
	}
	return nil
}

func splitRow(line string) []interface{} {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	var cells []interface{}
	var cell strings.Builder
	for index := 0; index < len(line); index++ {
		if line[index] == '\\' && index+1 < len(line) && line[index+1] == '|' {
			cell.WriteByte('|')
			index++
			continue
		}
		if line[index] == '|' {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
			continue
		}
		cell.WriteByte(line[index])
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

// ---------------------------------------------------------------- helpers

// blocks splits lines into blank-line-separated groups, each line trimmed.
func blocks(lines []string) [][]string {
	var out [][]string
	var current []string
	for _, line := range append(lines, "") {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				out = append(out, current)
				current = nil
			}
			continue
		}
		current = append(current, strings.TrimSpace(line))
	}
	return out
}

func nonBlank(lines []string) []string {
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimRight(line, " \t"))
		}
	}
	return out
}

// collapse joins a string's words with single spaces. It is the plain-text
// counterpart of oneLine, which also escapes for LaTeX. Only ASCII whitespace
// separates words: the SRDs set U+202F (narrow no-break space) before numbers
// on purpose, and strings.Fields would turn it into a plain space.
func collapse(s string) string {
	return strings.Join(strings.FieldsFunc(s, asciiSpace), " ")
}

func asciiSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
}
