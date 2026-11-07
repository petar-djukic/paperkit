// Command srd2md converts a paper's SRDs and VISION from YAML to the markdown
// form (srd/FORMAT.md), proving each conversion before it writes anything:
// the markdown is parsed back and must equal the YAML document, field for
// field, once whitespace is collapsed. Run it from the paper directory:
//
//	go run github.com/petar-djukic/paperkit/cmd/srd2md [-docs docs] [-check]
//
// -check verifies without writing. Otherwise each verified document is
// written beside its YAML and the YAML is removed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/petar-djukic/paperkit/srd"
)

func main() {
	docs := flag.String("docs", "docs", "paper docs directory holding VISION.yaml and srd/")
	check := flag.Bool("check", false, "verify every conversion without writing")
	flag.Parse()

	sources, _ := filepath.Glob(filepath.Join(*docs, "srd", "srd-*.yaml"))
	sort.Strings(sources)
	if vision := filepath.Join(*docs, "VISION.yaml"); exists(vision) {
		sources = append(sources, vision)
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "srd2md: no YAML SRDs or VISION under", *docs)
		os.Exit(1)
	}

	converted := map[string][]byte{}
	failed := 0
	for _, source := range sources {
		markdown, err := convert(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "srd2md: %s: %v\n", source, err)
			failed++
			continue
		}
		converted[source] = markdown
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "srd2md: %d of %d documents did not convert; nothing written\n", failed, len(sources))
		os.Exit(1)
	}
	if *check {
		fmt.Printf("srd2md: %d documents convert and round-trip\n", len(sources))
		return
	}
	for _, source := range sources {
		target := strings.TrimSuffix(source, ".yaml") + ".md"
		if exists(target) {
			fmt.Fprintf(os.Stderr, "srd2md: %s already exists; nothing more written\n", target)
			os.Exit(1)
		}
		if err := os.WriteFile(target, converted[source], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "srd2md:", err)
			os.Exit(1)
		}
		if err := os.Remove(source); err != nil {
			fmt.Fprintln(os.Stderr, "srd2md:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("srd2md: converted %d documents\n", len(sources))
}

// convert renders one document and proves the markdown reads back as the
// YAML did.
func convert(source string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	kind := srd.KindOf(source)
	markdown, err := srd.ToMarkdown(data, kind)
	if err != nil {
		return nil, err
	}
	var original map[string]interface{}
	if err := yaml.Unmarshal(data, &original); err != nil {
		return nil, err
	}
	parsed, err := srd.ParseMarkdown(markdown, kind)
	if err != nil {
		return nil, fmt.Errorf("the markdown does not parse: %w", err)
	}
	if path, ok := firstDifference("", srd.Normalize(original), srd.Normalize(parsed)); !ok {
		return nil, fmt.Errorf("the markdown reads back differently at %s", path)
	}
	return markdown, nil
}

// firstDifference reports the first path where two decoded documents differ.
func firstDifference(path string, a, b interface{}) (string, bool) {
	switch left := a.(type) {
	case map[string]interface{}:
		right, ok := b.(map[string]interface{})
		if !ok {
			return describe(path, a, b), false
		}
		keys := map[string]bool{}
		for key := range left {
			keys[key] = true
		}
		for key := range right {
			keys[key] = true
		}
		names := make([]string, 0, len(keys))
		for key := range keys {
			names = append(names, key)
		}
		sort.Strings(names)
		for _, key := range names {
			if p, ok := firstDifference(path+"."+key, left[key], right[key]); !ok {
				return p, false
			}
		}
		return "", true
	case []interface{}:
		right, ok := b.([]interface{})
		if !ok || len(left) != len(right) {
			return describe(path, a, b), false
		}
		for index := range left {
			if p, ok := firstDifference(fmt.Sprintf("%s[%d]", path, index), left[index], right[index]); !ok {
				return p, false
			}
		}
		return "", true
	}
	if reflect.DeepEqual(a, b) {
		return "", true
	}
	return describe(path, a, b), false
}

func describe(path string, a, b interface{}) string {
	return fmt.Sprintf("%s: YAML %s, markdown %s", strings.TrimPrefix(path, "."), short(a), short(b))
}

func short(v interface{}) string {
	text := fmt.Sprintf("%#v", v)
	if len(text) > 120 {
		text = text[:117] + "..."
	}
	return text
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
