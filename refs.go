package paperkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"gopkg.in/yaml.v3"
)

// ReferenceDrift lists the ids whose entry in the paper's bibliography differs
// from the entry of the same id in the corpus paper.yaml names. It returns
// nothing when paper.yaml names no corpus or the corpus is not on disk, which
// is the state of a copied-out paper.
//
// Drift is reported, never repaired: either side may be the better record (a
// paper's copy is often the one curated for print), so which entry wins is
// the author's decision, made in the corpus.
func ReferenceDrift(root string, config Config) ([]string, error) {
	if config.Corpus == "" {
		return nil, nil
	}
	corpus, err := readReferences(filepath.Join(root, config.Corpus))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paper, err := readReferences(filepath.Join(root, config.Bibliography))
	if err != nil {
		return nil, err
	}
	var drift []string
	for _, entry := range paper.entries {
		id := entryID(entry)
		shared, ok := corpus.byID[id]
		if !ok {
			continue
		}
		same, err := sameEntry(entry, shared)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if !same {
			drift = append(drift, id)
		}
	}
	sort.Strings(drift)
	return drift, nil
}

type references struct {
	entries []*yaml.Node
	byID    map[string]*yaml.Node
}

func readReferences(path string) (references, error) {
	var refs references
	data, err := os.ReadFile(path)
	if err != nil {
		return refs, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return refs, fmt.Errorf("parse %s: %w", path, err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.SequenceNode {
		return refs, fmt.Errorf("%s is not a CSL-YAML list of references", path)
	}
	refs.entries = document.Content[0].Content
	refs.byID = make(map[string]*yaml.Node, len(refs.entries))
	for _, entry := range refs.entries {
		if id := entryID(entry); id != "" {
			refs.byID[id] = entry
		}
	}
	return refs, nil
}

// entryID returns the value of an entry's id key, or "" for a node that is not
// a mapping or carries no id.
func entryID(entry *yaml.Node) string {
	if entry.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(entry.Content); index += 2 {
		if entry.Content[index].Value == "id" {
			return entry.Content[index+1].Value
		}
	}
	return ""
}

// sameEntry compares two entries by content, so quoting or layout that differs
// between the files does not count as drift.
func sameEntry(a, b *yaml.Node) (bool, error) {
	var left, right interface{}
	if err := a.Decode(&left); err != nil {
		return false, err
	}
	if err := b.Decode(&right); err != nil {
		return false, err
	}
	return reflect.DeepEqual(left, right), nil
}
