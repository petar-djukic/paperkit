// Command refs2bib generates the tracked BibTeX bibliography from the
// CSL-YAML corpus.
//
// references.yaml (managed by the update-references literature pipeline) is
// the source of truth. Each per-manuscript directory (e.g. ieee-proceedings/,
// ieee-comst/) keeps a tracked, regenerable references.bib so the LaTeX build
// can use bibtex/IEEEtran.bst and reviewers can diff the corpus.
//
// The corpus stores dates as `issued: {year: 2024}` (a shorthand pandoc
// accepts when reading a CSL-YAML bibliography, but not via its csljson
// reader). We normalize those to CSL date-parts before handing off to pandoc,
// otherwise years serialize as 0000. Organization/mononym authors stored as
// `{family: OpenAI}` are promoted to `{literal: OpenAI}` so IEEEtran.bst does
// not reject them.
//
// Usage (from a manuscript directory):
//
//	go run ../cmd/refs2bib --yaml ../references.yaml --out references.bib
//
// Requires pandoc on PATH.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var dateFields = []string{"issued", "original-date", "accessed", "submitted", "event-date"}

var nameFields = []string{
	"author", "editor", "translator", "container-author", "collection-editor",
	"director", "recipient", "interviewer", "original-author",
}

// normalizeNames promotes family-only names to CSL literal so pandoc braces
// them in the BibTeX output.
func normalizeNames(v interface{}) interface{} {
	names, ok := v.([]interface{})
	if !ok {
		return v
	}
	out := make([]interface{}, 0, len(names))
	for _, n := range names {
		m, ok := n.(map[string]interface{})
		if ok && truthy(m["family"]) && !truthy(m["given"]) && !truthy(m["literal"]) {
			out = append(out, map[string]interface{}{"literal": m["family"]})
		} else {
			out = append(out, n)
		}
	}
	return out
}

// truthy mirrors Python's dict.get(k) coupled with `if value` semantics for
// the shapes that occur in name entries (missing, nil, or string).
func truthy(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return t != ""
	default:
		return true
	}
}

// normalizeDate converts {year, month, day} shorthand to CSL date-parts.
func normalizeDate(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	if _, has := m["date-parts"]; has {
		return v
	}
	parts := []interface{}{}
	for _, k := range []string{"year", "month", "day"} {
		if val, has := m[k]; has && val != nil {
			parts = append(parts, val)
		}
	}
	if len(parts) == 0 {
		return v
	}
	normalized := map[string]interface{}{"date-parts": []interface{}{parts}}
	for _, passthrough := range []string{"season", "circa", "literal", "raw"} {
		if val, has := m[passthrough]; has {
			normalized[passthrough] = val
		}
	}
	return normalized
}

func main() {
	yamlPath := flag.String("yaml", "references.yaml", "CSL-YAML corpus to read")
	outPath := flag.String("out", "references.bib", "BibTeX file to write")
	flag.Parse()

	data, err := os.ReadFile(*yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refs2bib: corpus not found: %s\n", *yamlPath)
		os.Exit(2)
	}

	var entries []map[string]interface{}
	if err := yaml.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "refs2bib: %s is not a CSL-YAML list of references: %v\n", *yamlPath, err)
		os.Exit(2)
	}

	for _, entry := range entries {
		for _, field := range dateFields {
			if v, has := entry[field]; has {
				entry[field] = normalizeDate(v)
			}
		}
		for _, field := range nameFields {
			if v, has := entry[field]; has {
				entry[field] = normalizeNames(v)
			}
		}
	}

	tmp, err := os.CreateTemp("", "refs2bib-*.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "refs2bib:", err)
		os.Exit(1)
	}
	defer os.Remove(tmp.Name())
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(entries); err != nil {
		fmt.Fprintln(os.Stderr, "refs2bib:", err)
		os.Exit(1)
	}
	tmp.Close()

	if dir := filepath.Dir(*outPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "refs2bib:", err)
			os.Exit(1)
		}
	}
	cmd := exec.Command("pandoc", "-f", "csljson", "-t", "bibtex", tmp.Name(), "-o", *outPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "refs2bib: pandoc:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d entries) from %s\n", *outPath, len(entries), *yamlPath)
}
