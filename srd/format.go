package srd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The markdown form of an SRD or VISION document is defined in FORMAT.md.
// The reader returns the same map the YAML decoder returns, so everything
// downstream of Load works on either form unchanged.

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
	// locked fields are contract values: the writer wraps them in a block
	// lock so prose tooling, which treats a lock region as opaque, never
	// rewrites them.
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

// Load reads an SRD or VISION document in either form, by extension.
func Load(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".md") {
		doc, err := ParseMarkdown(data, KindOf(path))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return doc, nil
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// ---------------------------------------------------------------- writing

// ToMarkdown renders a YAML SRD or VISION document in the markdown form. It
// works from the YAML node tree, so key order and comments survive: each
// comment lands in an HTML comment under the heading of the field it sat in.
func ToMarkdown(source []byte, kind DocKind) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("not a YAML mapping")
	}
	root := document.Content[0]

	var out bytes.Buffer
	header := []string{document.HeadComment, root.HeadComment}
	var meta *yaml.Node
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == "meta" {
			meta = root.Content[index+1]
			header = append(header, root.Content[index].HeadComment)
			root.Content[index].HeadComment = ""
		}
	}
	if meta == nil {
		return nil, fmt.Errorf("no meta block")
	}
	frontMatter, err := encodeBlock(meta)
	if err != nil {
		return nil, err
	}
	out.WriteString("---\n")
	out.Write(frontMatter)
	out.WriteString("---\n")
	writeComment(&out, header...)

	for index := 0; index+1 < len(root.Content); index += 2 {
		key, value := root.Content[index], root.Content[index+1]
		if key.Value == "meta" {
			continue
		}
		spec, ok := specFor(kind, key.Value)
		if !ok {
			return nil, fmt.Errorf("field %q is not in the format (FORMAT.md)", key.Value)
		}
		var decoded interface{}
		if err := value.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("%s: %w", key.Value, err)
		}
		body, err := renderField(spec, decoded)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key.Value, err)
		}
		fmt.Fprintf(&out, "\n<!-- field: %s -->\n## %s\n", spec.name, spec.heading)
		writeComment(&out, collectComments(key, value)...)
		if body == "" {
			continue
		}
		out.WriteString("\n")
		if spec.locked {
			out.WriteString("<!-- lock -->\n" + body + "<!-- /lock -->\n")
		} else {
			out.WriteString(body)
		}
	}
	return out.Bytes(), nil
}

// encodeBlock writes a mapping as block-style YAML, whatever style the source
// used, so the front matter reads one key per line.
func encodeBlock(node *yaml.Node) ([]byte, error) {
	var setBlock func(*yaml.Node)
	setBlock = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			n.Style = 0
		}
		for _, child := range n.Content {
			setBlock(child)
		}
	}
	setBlock(node)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func collectComments(nodes ...*yaml.Node) []string {
	var comments []string
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		comments = append(comments, n.HeadComment, n.LineComment)
		for _, child := range n.Content {
			walk(child)
		}
		comments = append(comments, n.FootComment)
	}
	for _, n := range nodes {
		walk(n)
	}
	return comments
}

// writeComment emits YAML comments as one HTML comment, without their #.
func writeComment(out *bytes.Buffer, comments ...string) {
	var lines []string
	for _, comment := range comments {
		for _, line := range strings.Split(comment, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			lines = append(lines, strings.ReplaceAll(line, "-->", "->"))
		}
	}
	if len(lines) == 0 {
		return
	}
	out.WriteString("\n<!--\n" + strings.Join(lines, "\n") + "\n-->\n")
}

func renderField(spec fieldSpec, value interface{}) (string, error) {
	var b strings.Builder
	switch spec.kind {
	case proseField:
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("want text, have %T", value)
		}
		for index, paragraph := range paragraphs(text) {
			if index > 0 {
				b.WriteString("\n")
			}
			b.WriteString(paragraph + "\n")
		}
	case listField:
		items, err := asList(value)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("want a list of text, have a %T item", item)
			}
			b.WriteString("- " + collapse(text) + "\n")
		}
	case goalsField:
		records, err := asRecords(value, "id", "goal")
		if err != nil {
			return "", err
		}
		for _, r := range records {
			b.WriteString("- **" + str(r["id"]) + "** " + collapse(str(r["goal"])) + "\n")
		}
	case citationsField:
		records, err := asRecords(value, "id", "role", "note")
		if err != nil {
			return "", err
		}
		for _, r := range records {
			b.WriteString("- **" + str(r["id"]) + "**")
			if role, ok := r["role"]; ok {
				b.WriteString(" (" + str(role) + ")")
			}
			if note, ok := r["note"]; ok {
				b.WriteString(":")
				if text := collapse(str(note)); text != "" {
					b.WriteString(" " + text)
				}
			}
			b.WriteString("\n")
		}
	case priorField:
		records, err := asRecords(value, "path", "offers")
		if err != nil {
			return "", err
		}
		for _, r := range records {
			b.WriteString("- `" + str(r["path"]) + "`: " + collapse(str(r["offers"])) + "\n")
		}
	case contentField:
		records, err := asRecords(value, "say", "carried_by", "artifact")
		if err != nil {
			return "", err
		}
		for index, r := range records {
			say := collapse(str(r["say"]))
			if err := checkSay(say); err != nil {
				return "", fmt.Errorf("item %d: %w", index+1, err)
			}
			if index > 0 {
				b.WriteString("\n")
			}
			b.WriteString(say)
			if carried, ok := r["carried_by"]; ok {
				ids, err := asStrings(carried)
				if err != nil {
					return "", fmt.Errorf("item %d: carried_by: %w", index+1, err)
				}
				if len(ids) == 0 {
					return "", fmt.Errorf("item %d: carried_by is empty; remove the key", index+1)
				}
				b.WriteString(" " + citeGroup(ids))
			}
			b.WriteString("\n")
			if artifact, ok := r["artifact"]; ok {
				b.WriteString("<!-- artifact: " + strings.ReplaceAll(collapse(str(artifact)), "-->", "->") + " -->\n")
			}
		}
	case linksField, inputsField:
		fields, ok := value.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("want a mapping, have %T", value)
		}
		for _, name := range sortedKeys(fields) {
			if spec.kind == linksField {
				ids, err := asStrings(fields[name])
				if err != nil {
					return "", fmt.Errorf("%s: %w", name, err)
				}
				b.WriteString(strings.TrimRight("- "+name+": "+strings.Join(ids, ", "), " ") + "\n")
			} else {
				b.WriteString(strings.TrimRight("- "+name+": "+collapse(str(fields[name])), " ") + "\n")
			}
		}
	case tablesField:
		records, err := asRecords(value, "shows", "columns", "rows", "status", "wide")
		if err != nil {
			return "", err
		}
		for index, r := range records {
			if index > 0 {
				b.WriteString("\n")
			}
			if err := renderTable(&b, r); err != nil {
				return "", fmt.Errorf("table %d: %w", index+1, err)
			}
		}
	case figuresField:
		records, err := asRecords(value, "file", "status", "shows", "wide")
		if err != nil {
			return "", err
		}
		for _, r := range records {
			b.WriteString("- `" + str(r["file"]) + "`")
			if attrs := attributes(r, "status", "wide"); attrs != "" {
				b.WriteString(" (" + attrs + ")")
			}
			b.WriteString(": " + collapse(str(r["shows"])) + "\n")
		}
	}
	return b.String(), nil
}

func renderTable(b *strings.Builder, r map[string]interface{}) error {
	attrs := attributes(r, "status", "wide")
	for _, key := range []string{"columns", "rows"} {
		if v, ok := r[key]; ok {
			if items, _ := v.([]interface{}); len(items) == 0 {
				attrs = joinAttr(attrs, key+": []")
			}
		}
	}
	if attrs == "" {
		b.WriteString("<!-- table -->\n")
	} else {
		b.WriteString("<!-- table | " + strings.ReplaceAll(attrs, ", ", " | ") + " -->\n")
	}
	if shows, ok := r["shows"]; ok {
		b.WriteString("Table: " + collapse(str(shows)) + "\n")
	}
	columns, err := asStrings(r["columns"])
	if err != nil {
		return fmt.Errorf("columns: %w", err)
	}
	if len(columns) == 0 {
		return nil
	}
	b.WriteString("\n" + tableRow(columns) + "\n|" + strings.Repeat("---|", len(columns)) + "\n")
	rows, err := asList(r["rows"])
	if err != nil {
		return fmt.Errorf("rows: %w", err)
	}
	for _, row := range rows {
		cells, err := asStrings(row)
		if err != nil {
			return fmt.Errorf("row: %w", err)
		}
		b.WriteString(tableRow(cells) + "\n")
	}
	return nil
}

func tableRow(cells []string) string {
	escaped := make([]string, len(cells))
	for index, cell := range cells {
		escaped[index] = strings.ReplaceAll(collapse(cell), "|", `\|`)
	}
	return "| " + strings.Join(escaped, " | ") + " |"
}

// attributes renders the named keys a record carries as "key: value" pairs.
func attributes(r map[string]interface{}, keys ...string) string {
	var attrs string
	for _, key := range keys {
		if v, ok := r[key]; ok {
			attrs = joinAttr(attrs, key+": "+fmt.Sprint(v))
		}
	}
	return attrs
}

func joinAttr(attrs, attr string) string {
	if attrs == "" {
		return attr
	}
	return attrs + ", " + attr
}

func citeGroup(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return "[@" + strings.Join(ids, "; @") + "]"
}

// ---------------------------------------------------------------- reading

var (
	fieldTag     = regexp.MustCompile(`^<!-- field: ([a-z_]+) -->$`)
	artifactTag  = regexp.MustCompile(`^<!-- artifact: (.*) -->$`)
	citeGroupRe  = regexp.MustCompile(`\s*\[@[^\]]*\]`)
	sayMarker    = regexp.MustCompile(`^(- |\* |\+ |[0-9]+\. |#|>|\||<!--)`)
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

// ---------------------------------------------------------------- equality

// Normalize returns a copy of a decoded document with every string's
// whitespace collapsed to single spaces. The markdown form keeps text, not
// line layout, so two forms of one document are equal exactly when their
// normalized maps are; every consumer reads the text through oneLine anyway.
func Normalize(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		return collapse(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for index, item := range t {
			out[index] = Normalize(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for key, item := range t {
			out[key] = Normalize(item)
		}
		return out
	}
	return v
}

// ---------------------------------------------------------------- helpers

var blankLine = regexp.MustCompile(`\n\s*\n`)

func paragraphs(text string) []string {
	var out []string
	for _, block := range blankLine.Split(strings.TrimSpace(text), -1) {
		if line := collapse(block); line != "" {
			out = append(out, line)
		}
	}
	return out
}

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

// checkSay rejects a say the markdown form could not carry unambiguously: one
// holding its own citation group, whose carried_by would absorb it, or one
// opening with a list, heading, quote, table or comment marker.
func checkSay(say string) error {
	if strings.Contains(say, "[@") {
		return fmt.Errorf("say holds a [@...] citation group; move the ids to carried_by")
	}
	if sayMarker.MatchString(say) {
		return fmt.Errorf("say opens with a markdown marker: %.40q", say)
	}
	return nil
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

func asList(v interface{}) ([]interface{}, error) {
	if v == nil {
		return nil, nil
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("want a list, have %T", v)
	}
	return items, nil
}

func asStrings(v interface{}) ([]string, error) {
	items, err := asList(v)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("want text, have a %T item", item)
		}
		out[index] = text
	}
	return out, nil
}

// asRecords reads a list of mappings whose keys are all among allowed.
func asRecords(v interface{}, allowed ...string) ([]map[string]interface{}, error) {
	items, err := asList(v)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, len(items))
	for index, item := range items {
		record, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("item %d: want a mapping, have %T", index+1, item)
		}
		for key := range record {
			if !contains(allowed, key) {
				return nil, fmt.Errorf("item %d: key %q is not in the format (FORMAT.md)", index+1, key)
			}
		}
		out[index] = record
	}
	return out, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
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
