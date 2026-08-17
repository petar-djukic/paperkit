package paperkit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// baselineDirName holds the snapshot of what generation last produced for each
// chapter. It is committed rather than ignored: it is the only record of the
// generated-from state, and without it a fresh clone cannot tell a hand edit
// from generated text, so regeneration would silently overwrite the author's
// typesetting.
const baselineDirName = ".baseline"

// Disposition is what reconciling one chapter did to its LaTeX file.
type Disposition int

const (
	// Created means the chapter had no LaTeX yet; the generated version was
	// written as-is.
	Created Disposition = iota
	// Replaced means the LaTeX matched its baseline, so it carried no hand
	// edits and the generated version replaced it.
	Replaced
	// Merged means hand edits were present and the regenerated prose merged
	// into them cleanly.
	Merged
	// Conflicted means hand edits and regenerated prose touched the same
	// region. The file carries conflict markers and needs the author.
	Conflicted
	// Orphaned means the chapter has LaTeX but no baseline, so whether it
	// carries hand edits cannot be determined. Nothing was written.
	Orphaned
)

func (d Disposition) String() string {
	switch d {
	case Created:
		return "created"
	case Replaced:
		return "replaced"
	case Merged:
		return "merged"
	case Conflicted:
		return "conflicted"
	case Orphaned:
		return "orphaned"
	}
	return "unknown"
}

// ChapterResult reports what reconciling one chapter did.
type ChapterResult struct {
	Chapter     string
	Disposition Disposition
}

// ErrConflicts reports that at least one chapter needs the author to resolve
// conflict markers, or could not be reconciled at all.
var ErrConflicts = errors.New("chapters need attention")

// baselinePath returns where a chapter's last-generated snapshot lives.
func baselinePath(root string, config Config, chapter string) string {
	return filepath.Join(root, config.TexDir, baselineDirName, TexName(chapter))
}

func texPath(root string, config Config, chapter string) string {
	return filepath.Join(root, config.TexDir, TexName(chapter))
}

// reconcile decides what the generated LaTeX for one chapter does to the
// author's copy, and writes the outcome.
//
// The four states, by whether the author's file and the baseline exist:
//
//   - no author file: first generation for this chapter, write it.
//   - author file, no baseline: refuse. Without the baseline there is no way
//     to tell hand-edited typesetting from generated text, and overwriting
//     would destroy work silently. This is the one case that reports rather
//     than acts.
//   - author file identical to baseline: no hand edits, replace it.
//   - author file differs from baseline: three-way merge.
func reconcile(root string, config Config, chapter string, generated []byte) (Disposition, error) {
	author := texPath(root, config, chapter)
	baseline := baselinePath(root, config, chapter)

	authorContent, authorErr := os.ReadFile(author)
	if errors.Is(authorErr, os.ErrNotExist) {
		return Created, writeChapter(author, baseline, generated, generated)
	}
	if authorErr != nil {
		return Created, authorErr
	}

	baselineContent, baselineErr := os.ReadFile(baseline)
	if errors.Is(baselineErr, os.ErrNotExist) {
		return Orphaned, nil
	}
	if baselineErr != nil {
		return Orphaned, baselineErr
	}

	if bytes.Equal(authorContent, baselineContent) {
		return Replaced, writeChapter(author, baseline, generated, generated)
	}

	merged, conflicts, err := mergeFile(authorContent, baselineContent, generated)
	if err != nil {
		return Conflicted, err
	}
	// The baseline advances to the generated content even on conflict. It
	// records what generation last produced, not what the author accepted, so
	// once the markers are resolved the next run sees no pending upstream
	// change and leaves the resolution alone.
	if err := writeChapter(author, baseline, merged, generated); err != nil {
		return Conflicted, err
	}
	if conflicts > 0 {
		return Conflicted, nil
	}
	return Merged, nil
}

func writeChapter(author, baseline string, content, baselineContent []byte) error {
	if err := os.MkdirAll(filepath.Dir(baseline), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(author, content, 0o644); err != nil {
		return err
	}
	return os.WriteFile(baseline, baselineContent, 0o644)
}

// mergeFile three-way merges through `git merge-file`, which is already a hard
// dependency of every workflow here and emits the conflict markers the author
// resolves daily. It returns the merged bytes and the number of conflicts.
func mergeFile(ours, base, theirs []byte) ([]byte, int, error) {
	dir, err := os.MkdirTemp("", "paperkit-merge-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)

	paths := map[string][]byte{"ours": ours, "base": base, "theirs": theirs}
	for name, content := range paths {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return nil, 0, err
		}
	}

	command := exec.Command("git", "merge-file", "-p",
		"-L", "hand-edited", "-L", "last generated", "-L", "regenerated",
		filepath.Join(dir, "ours"),
		filepath.Join(dir, "base"),
		filepath.Join(dir, "theirs"),
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err == nil {
		return stdout.Bytes(), 0, nil
	}
	// git merge-file exits with the conflict count, and negative on failure.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return stdout.Bytes(), exitErr.ExitCode(), nil
	}
	return nil, 0, fmt.Errorf("git merge-file: %w: %s", err, stderr.String())
}
