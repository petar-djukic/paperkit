package paperkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GenerateTex writes one LaTeX fragment per chapter into the paper's tex
// directory, plus a main.tex container that inputs them in config order.
//
// Chapters are generated as fragments rather than standalone documents so the
// author can hand-edit a chapter's floats without touching the preamble, and
// so regenerating one chapter cannot disturb another.
//
// Regeneration goes through reconcile, so LaTeX the author has hand-edited is
// merged rather than overwritten. It returns one ChapterResult per chapter, and
// when any chapter conflicts or cannot be reconciled the error wraps
// ErrConflicts so a caller can report the detail and still fail the build.
func GenerateTex(root string, config Config) ([]ChapterResult, error) {
	texDir := filepath.Join(root, config.TexDir)
	if err := os.MkdirAll(texDir, 0o755); err != nil {
		return nil, err
	}

	return generateWithLibrary(root, config)
}

// generateWithLibrary converts every source first, checks the labels across
// the whole roster, and only then lets anything reach the author's tex
// directory.
func generateWithLibrary(root string, config Config) ([]ChapterResult, error) {
	fragments, converted, err := convertRoster(root, config)
	if err != nil {
		return nil, err
	}
	if err := checkCollisions(converted); err != nil {
		return nil, err
	}

	results, err := reconcileAll(root, config, config.roster(), func(source string) ([]byte, error) {
		return fragments[source], nil
	})
	if err != nil {
		return results, err
	}
	if err := generateContainer(root, config); err != nil {
		return results, err
	}
	return results, attention(results)
}

// reconcileAll renders each source and merges it into the author's copy,
// reporting what happened to every one of them. Rendering is the caller's,
// because that is the only part the two engines do differently.
func reconcileAll(root string, config Config, sources []string, render func(string) ([]byte, error)) ([]ChapterResult, error) {
	results := make([]ChapterResult, 0, len(sources))
	for _, source := range sources {
		generated, err := render(source)
		if err != nil {
			return results, fmt.Errorf("%s: %w", source, err)
		}
		disposition, err := reconcile(root, config, source, generated)
		if err != nil {
			return results, fmt.Errorf("%s: %w", source, err)
		}
		results = append(results, ChapterResult{Chapter: source, Disposition: disposition})
	}
	return results, nil
}

// attention reports the chapters a person has to look at, so a build that
// merged cleanly is silent and one that did not fails with a count.
func attention(results []ChapterResult) error {
	needsAttention := 0
	for _, result := range results {
		if result.Disposition == Conflicted || result.Disposition == Orphaned {
			needsAttention++
		}
	}
	if needsAttention > 0 {
		return fmt.Errorf("%d of %d chapters: %w", needsAttention, len(results), ErrConflicts)
	}
	return nil
}

func runIn(dir, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
