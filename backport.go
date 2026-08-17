package paperkit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// BackportDisposition is what a chapter's LaTeX had to say about its markdown.
type BackportDisposition int

const (
	// Untouched means the chapter's LaTeX matches its baseline, so there is
	// nothing to carry back.
	Untouched BackportDisposition = iota
	// TypesetOnly means the LaTeX was edited but the edit does not survive
	// conversion back to markdown, which is what a pure typesetting change
	// looks like: float placement, column hints, table layout.
	TypesetOnly
	// Proposed means the LaTeX carries a prose change that merges into the
	// markdown source.
	Proposed
	// Contested means the prose change and the markdown source disagree; the
	// author has to resolve it in the markdown.
	Contested
	// Unbased means the chapter has LaTeX but no baseline, so there is nothing
	// to diff the edit against.
	Unbased
)

func (b BackportDisposition) String() string {
	switch b {
	case Untouched:
		return "untouched"
	case TypesetOnly:
		return "typesetting only"
	case Proposed:
		return "prose change proposed"
	case Contested:
		return "contested"
	case Unbased:
		return "no baseline"
	}
	return "unknown"
}

// BackportResult is one chapter's proposed change to its markdown source.
type BackportResult struct {
	Chapter     string
	Disposition BackportDisposition
	// Diff is a unified diff of the proposed markdown change, for the author
	// to read before deciding.
	Diff string
	// Markdown is the merged source. It is written only by ApplyBackport, so
	// computing a proposal never touches the working tree.
	Markdown []byte
}

// Backport works out what each chapter's hand-edited LaTeX implies for its
// markdown source. It writes nothing: markdown is the source of truth, the
// tex-to-markdown conversion is lossy, and applying pandoc's rendering
// wholesale would reformat chapters the author never touched. Pass the results
// to ApplyBackport once the author has read the diffs.
//
// Each chapter is compared in markdown space rather than LaTeX space:
//
//   - base   = the baseline LaTeX converted back to markdown,
//   - theirs = the author's current LaTeX converted back to markdown,
//   - ours   = the markdown source on disk.
//
// Converting both sides through the same lossy path is what isolates the
// author's prose edit: whatever the conversion mangles, it mangles identically
// on both sides, so it cancels. Presentation pandoc does preserve is stripped
// first (see stripLatexAttributes), which leaves a pure typesetting edit with
// nothing to propose.
func Backport(root string, config Config) ([]BackportResult, error) {
	results := make([]BackportResult, 0, len(config.Chapters))
	contested := 0

	for _, chapter := range config.Chapters {
		result, err := backportChapter(root, config, chapter)
		if err != nil {
			return results, fmt.Errorf("%s: %w", chapter, err)
		}
		results = append(results, result)
		if result.Disposition == Contested {
			contested++
		}
	}

	if contested > 0 {
		return results, fmt.Errorf("%d chapter(s) contested: %w", contested, ErrConflicts)
	}
	return results, nil
}

func backportChapter(root string, config Config, chapter string) (BackportResult, error) {
	result := BackportResult{Chapter: chapter, Disposition: Untouched}

	authored, err := os.ReadFile(texPath(root, config, chapter))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	baseline, err := os.ReadFile(baselinePath(root, config, chapter))
	if errors.Is(err, os.ErrNotExist) {
		result.Disposition = Unbased
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if bytes.Equal(authored, baseline) {
		return result, nil
	}

	baseMarkdown, err := texToMarkdown(baseline)
	if err != nil {
		return result, fmt.Errorf("convert baseline: %w", err)
	}
	theirMarkdown, err := texToMarkdown(authored)
	if err != nil {
		return result, fmt.Errorf("convert edited tex: %w", err)
	}
	baseMarkdown = stripLatexAttributes(baseMarkdown)
	theirMarkdown = stripLatexAttributes(theirMarkdown)
	// A typesetting-only edit converts back to the same markdown on both
	// sides, leaving nothing to propose.
	if bytes.Equal(baseMarkdown, theirMarkdown) {
		result.Disposition = TypesetOnly
		return result, nil
	}

	source, err := os.ReadFile(filepath.Join(root, chapter))
	if err != nil {
		return result, err
	}
	merged, conflicts, err := mergeFile(source, baseMarkdown, theirMarkdown)
	if err != nil {
		return result, err
	}
	result.Markdown = merged
	if conflicts > 0 {
		result.Disposition = Contested
	} else {
		result.Disposition = Proposed
	}
	diff, err := unifiedDiff(chapter, source, merged)
	if err != nil {
		return result, err
	}
	result.Diff = diff
	return result, nil
}

// ApplyBackport writes the proposed markdown for every chapter that has one.
// It is deliberately separate from Backport so that nothing reaches the
// working tree until the author has seen the diffs.
func ApplyBackport(root string, results []BackportResult) error {
	var applied []error
	for _, result := range results {
		if result.Disposition != Proposed || len(result.Markdown) == 0 {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, result.Chapter), result.Markdown, 0o644); err != nil {
			applied = append(applied, fmt.Errorf("%s: %w", result.Chapter, err))
		}
	}
	return errors.Join(applied...)
}

// latexAttribute matches the attributes pandoc uses to carry LaTeX-only
// presentation through a round trip, such as data-latex-placement="!t" on a
// float.
var latexAttribute = regexp.MustCompile(` data-latex[a-z-]*="[^"]*"`)

// stripLatexAttributes removes those attributes before the two sides are
// compared.
//
// Pandoc does not discard float placement on the way back to markdown; it
// preserves it as an attribute. Without this, retypesetting a float reads as a
// prose change and the proposal writes <figure data-latex-placement="!t"> into
// the markdown — putting typesetting into the source of truth, and putting
// markup the author never wrote in front of them in Obsidian. Placement
// belongs in tex/, so it is normalized out of both sides and never proposed.
func stripLatexAttributes(markdown []byte) []byte {
	return latexAttribute.ReplaceAll(markdown, nil)
}

// texToMarkdown converts LaTeX back to markdown. --wrap=preserve keeps pandoc
// from rewrapping paragraphs, which would otherwise make every line differ and
// bury the prose change.
func texToMarkdown(tex []byte) ([]byte, error) {
	command := exec.Command("pandoc",
		"--from", "latex",
		"--to", "markdown",
		"--wrap=preserve",
	)
	command.Stdin = bytes.NewReader(tex)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("pandoc: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// unifiedDiff renders the change to a chapter for the author to read. git diff
// exits 1 when the files differ, which is the expected case here, so only a
// larger status counts as failure.
func unifiedDiff(name string, before, after []byte) (string, error) {
	dir, err := os.MkdirTemp("", "paperkit-diff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	// The two sides are written under directories named for what they are and
	// files named for the chapter, so the diff header reads
	// "current/06-interface-view.md" rather than a temp path the author has to
	// decode.
	base := filepath.Base(name)
	beforePath := filepath.Join("current", base)
	afterPath := filepath.Join("proposed", base)
	for path, content := range map[string][]byte{beforePath: before, afterPath: after} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return "", err
		}
	}

	command := exec.Command("git", "diff", "--no-index", "--no-color",
		"--src-prefix=", "--dst-prefix=",
		"--", beforePath, afterPath)
	command.Dir = dir
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() > 1 {
			return "", fmt.Errorf("git diff %s: %w", name, err)
		}
	}
	return stdout.String(), nil
}
