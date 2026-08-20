// Command refs2bib generates the tracked BibTeX bibliography from the
// CSL-YAML corpus.
//
// references.yaml (managed by the update-references literature pipeline) is
// the source of truth. Each per-manuscript directory (e.g. autonomous-network-tutorial/) keeps
// a tracked, regenerable references.bib so the LaTeX build can use
// bibtex/IEEEtran.bst and reviewers can diff the corpus.
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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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
			continue
		}
		out = append(out, n)
	}
	return out
}

// accentCommand maps a combining mark to the LaTeX accent that produces it.
var accentCommand = map[rune]string{
	0x0301: `\'`, 0x0300: "\\`", 0x0302: `\^`, 0x0308: `\"`, 0x0303: `\~`,
	0x0304: `\=`, 0x0306: `\u`, 0x0307: `\.`, 0x030A: `\r`, 0x030B: `\H`,
	0x030C: `\v`, 0x0327: `\c`, 0x0328: `\k`,
}

// strokedLetter covers the letters that carry no combining mark to decompose,
// so no accent command can be derived for them.
var strokedLetter = map[rune]string{
	'Ł': `\L`, 'ł': `\l`, 'Ø': `\O`, 'ø': `\o`, 'Đ': `\DJ`, 'đ': `\dj`,
	'Æ': `\AE`, 'æ': `\ae`, 'Œ': `\OE`, 'œ': `\oe`, 'ß': `\ss`,
}

// protectInitial rewrites a non-ASCII first character as its LaTeX accent
// form, so "Éric" becomes "{\'E}ric" and "Łukasz" becomes "{\L}ukasz".
//
// IEEEtranN.bst abbreviates a given name to its initial, and bibtex is
// byte-oriented: it takes the first byte of "Éric", half of a two-byte
// character, and the .bbl becomes invalid UTF-8. xelatex then reports `Invalid
// UTF-8 byte or sequence` and sets a replacement glyph where the initial
// belongs.
//
// Bracing the character alone does not help. bibtex treats a group as one
// token only when it opens with a control sequence, so {É} is still split byte
// by byte; {\'E} is not. That is why this converts rather than merely wraps.
//
// The conversion is derived from Unicode decomposition rather than a table of
// whole characters, so any letter that decomposes to a base plus a mark this
// map knows is handled without being listed. Anything left unconvertible is
// returned untouched, which is the behaviour before this existed.
func protectInitial(given string) string {
	trimmed := strings.TrimSpace(given)
	if trimmed == "" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, `\`) {
		return given
	}
	first, width := utf8.DecodeRuneInString(trimmed)
	command, ok := latexInitial(first)
	if !ok {
		return given
	}
	return command + trimmed[width:]
}

// latexInitial gives the braced LaTeX form of a single non-ASCII letter, and
// reports whether one could be derived.
func latexInitial(first rune) (string, bool) {
	if first == utf8.RuneError || first < utf8.RuneSelf {
		return "", false
	}
	if command, known := strokedLetter[first]; known {
		return "{" + command + "}", true
	}
	runes := []rune(norm.NFD.String(string(first)))
	if len(runes) == 2 {
		if command, known := accentCommand[runes[1]]; known {
			// A control sequence named with letters, \c or \v, needs a space
			// before the base letter or TeX reads them as one longer name.
			// One named with punctuation, \' or \", does not.
			separator := ""
			if last, _ := utf8.DecodeLastRuneInString(command); unicode.IsLetter(last) {
				separator = " "
			}
			return "{" + command + separator + string(runes[0]) + "}", true
		}
	}
	return "", false
}

// givenInitial matches the first character of a given name inside a BibTeX
// author list, where names are written "Family, Given" and separated by "and".
// Anchoring on the comma is what makes this work across a wrapped field: the
// separator between names may carry a newline, but "Family," never does.
var givenInitial = regexp.MustCompile(`,(\s+)([^\x00-\x7F])`)

// protectAccentedInitials rewrites the first character of every given name in
// the generated BibTeX from a non-ASCII letter to its LaTeX accent form.
//
// It runs on the file rather than on the CSL data because pandoc's bibtex
// writer escapes whatever it is handed: braces become \{ and \}, and a
// backslash becomes \textbackslash, so anything injected upstream arrives as
// literal text. After pandoc is the only place the substitution survives.
func protectAccentedInitials(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fixed := givenInitial.ReplaceAllFunc(data, func(match []byte) []byte {
		parts := givenInitial.FindSubmatch(match)
		first, _ := utf8.DecodeRune(parts[2])
		command, ok := latexInitial(first)
		if !ok {
			return match
		}
		return []byte("," + string(parts[1]) + command)
	})
	if bytes.Equal(fixed, data) {
		return nil
	}
	return os.WriteFile(path, fixed, 0o644)
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
	if err := protectAccentedInitials(*outPath); err != nil {
		fmt.Fprintln(os.Stderr, "refs2bib:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d entries) from %s\n", *outPath, len(entries), *yamlPath)
}
