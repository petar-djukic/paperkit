package paperkit

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateTex writes one LaTeX fragment per chapter into the paper's tex
// directory, plus a main.tex container that inputs them in config order.
//
// The tex directory is build output: markdown is the only source, and every
// run overwrites each fragment with what the conversion produced. Chapters are
// still generated as fragments rather than one document so the container can
// order them and a label collision names the chapters involved.
func GenerateTex(root string, config Config) error {
	texDir := filepath.Join(root, config.TexDir)
	if err := os.MkdirAll(texDir, 0o755); err != nil {
		return err
	}

	// Every source converts and the labels are checked across the whole roster
	// before anything is written, so a collision leaves tex/ as it was.
	fragments, converted, err := convertRoster(root, config)
	if err != nil {
		return err
	}
	if err := checkCollisions(converted); err != nil {
		return err
	}

	for _, source := range config.roster() {
		if err := os.WriteFile(texPath(root, config, source), fragments[source], 0o644); err != nil {
			return fmt.Errorf("%s: %w", source, err)
		}
	}
	return generateContainer(root, config)
}

func texPath(root string, config Config, chapter string) string {
	return filepath.Join(root, config.TexDir, TexName(chapter))
}
