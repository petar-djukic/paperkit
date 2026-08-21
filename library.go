package paperkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mdtotex "github.com/petar-djukic/md-to-tex"
	"gopkg.in/yaml.v3"
)

// ErrCollisions reports identifiers that more than one chapter claims. It is
// separate from ErrConflicts because the author resolves it differently: a
// conflict is a merge to settle in the LaTeX, a collision is a heading to name
// in the markdown.
var ErrCollisions = errors.New("chapters claim the same label")

// convertRoster renders every markdown source the container will input, and
// returns the fragments keyed by source name.
//
// Every source converts before anything is written. Conversion through the
// library is a pure call over bytes, so it costs nothing to do the whole
// roster first, and it buys the property that a paper with colliding labels
// leaves the author's tex directory untouched rather than half-regenerated.
func convertRoster(root string, config Config) (map[string][]byte, []mdtotex.Result, error) {
	keys, err := citationKeys(root, config)
	if err != nil {
		return nil, nil, err
	}
	options := mdtotex.Options{CitationKeys: keys}

	fragments := make(map[string][]byte, len(config.Chapters)+2)
	results := make([]mdtotex.Result, 0, len(config.Chapters))

	for _, source := range config.roster() {
		data, err := os.ReadFile(filepath.Join(root, source))
		if err != nil {
			return nil, nil, err
		}
		name := filepath.Base(source)

		// The title page carries metadata with destinations — \title, \author,
		// the abstract environment — so it renders through the front-matter
		// path rather than as a chapter.
		if source == config.TitlePage {
			result, err := mdtotex.RenderFrontMatter(data, name, options)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", source, err)
			}
			fragments[source] = result.LaTeX
			continue
		}

		result, err := mdtotex.Convert(data, name, options)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", source, err)
		}
		fragments[source] = result.LaTeX
		results = append(results, result)
	}
	return fragments, results, nil
}

// checkCollisions reports every identifier more than one chapter claims.
//
// The library refuses to disambiguate these itself, and paperkit must not do
// it either: prefixing a derived label with its chapter stem, which the pandoc
// path does, silently breaks any raw \ref another chapter writes against the
// unprefixed name. The author names one of the headings instead, which is why
// the message carries the headings rather than only the identifier.
func checkCollisions(results []mdtotex.Result) error {
	collisions := mdtotex.Collisions(results...)
	if len(collisions) == 0 {
		return nil
	}

	lines := make([]string, 0, len(collisions))
	for _, collision := range collisions {
		claims := make([]string, 0, len(collision.Carriers))
		for _, carrier := range collision.Carriers {
			claims = append(claims, fmt.Sprintf("%s (%s)", carrier.Chapter, carrier.Heading))
		}
		lines = append(lines, fmt.Sprintf("  %s: %s",
			collision.Identifier, strings.Join(claims, ", ")))
	}
	return fmt.Errorf("%w; state an identifier on one of each pair:\n%s",
		ErrCollisions, strings.Join(lines, "\n"))
}

// generateContainer writes tex/main.tex for the library path.
//
// The container carries no content: the class, the preamble it includes by
// name, the graphics paths, one input per roster entry, and the bibliography.
// Regenerating it cannot disturb a chapter, and the preamble stays the
// author's file.
func generateContainer(root string, config Config) error {
	graphics, err := graphicsPaths(config)
	if err != nil {
		return err
	}
	document, err := mdtotex.GenerateContainer(config.roster(), mdtotex.ContainerOptions{
		DocumentClass:     config.DocumentClass,
		ClassOptions:      config.ClassOptions,
		Preamble:          preambleInput(config),
		BibliographyStyle: "IEEEtranN",
		Bibliography:      "references",
		GraphicsPaths:     graphics,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, config.TexDir, "main.tex"), document, 0o644)
}

// preambleInput names the preamble as LaTeX must reach it. The configured path
// is relative to the paper directory and latexmk runs in the tex directory, so
// the input climbs back out; the extension goes because \input supplies it.
func preambleInput(config Config) string {
	relative := filepath.ToSlash(filepath.Join("..", config.Preamble))
	return strings.TrimSuffix(relative, filepath.Ext(relative))
}

// graphicsPaths points graphicx at the paper directory and its figure
// directory, both relative to where latexmk runs. Figures are included by base
// name, so these are what resolve them.
func graphicsPaths(config Config) ([]string, error) {
	toRoot, err := filepath.Rel(config.TexDir, ".")
	if err != nil {
		return nil, err
	}
	root := filepath.ToSlash(toRoot) + "/"
	return []string{root, root + filepath.ToSlash(config.FigureDir) + "/"}, nil
}

// citationKeys reads the ids the reference corpus defines, which is the set a
// citation may name. The library validates against it and reports an unknown
// key with the file it appears in, so a mistyped key fails at conversion
// rather than as an undefined citation in the LaTeX log.
func citationKeys(root string, config Config) ([]string, error) {
	path := filepath.Join(root, config.Bibliography)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reference corpus: %w", err)
	}
	var entries []struct {
		ID string `yaml:"id"`
	}
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse %s: %w", config.Bibliography, err)
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ID != "" {
			keys = append(keys, entry.ID)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s defines no reference ids", config.Bibliography)
	}
	return keys, nil
}
