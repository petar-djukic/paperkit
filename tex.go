package paperkit

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// beginDocument and bibliographyLine bracket the region of pandoc's standalone
// skeleton that holds the document body. The chapter inputs go between them.
const (
	beginDocument    = `\begin{document}`
	bibliographyLine = `\bibliography{references}`
)

// GenerateTex writes one LaTeX fragment per chapter into the paper's tex
// directory, plus a main.tex skeleton that inputs them in config order.
//
// Chapters are generated as fragments rather than standalone documents so the
// author can hand-edit a chapter's floats without touching the preamble, and
// so regenerating one chapter cannot disturb another. The skeleton comes from
// pandoc rather than a template of our own because pandoc's body output
// depends on helper macros it emits into its own preamble (\tightlist,
// longtable setup, \pandocbounded); a hand-written skeleton compiles until a
// chapter happens to use one of them.
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

	results := make([]ChapterResult, 0, len(config.Chapters))
	needsAttention := 0
	for _, chapter := range config.Chapters {
		generated, err := generateChapter(root, config, chapter)
		if err != nil {
			return results, fmt.Errorf("%s: %w", chapter, err)
		}
		disposition, err := reconcile(root, config, chapter, generated)
		if err != nil {
			return results, fmt.Errorf("%s: %w", chapter, err)
		}
		results = append(results, ChapterResult{Chapter: chapter, Disposition: disposition})
		if disposition == Conflicted || disposition == Orphaned {
			needsAttention++
		}
	}

	if err := generateSkeleton(root, config); err != nil {
		return results, err
	}
	if needsAttention > 0 {
		return results, fmt.Errorf("%d of %d chapters: %w", needsAttention, len(config.Chapters), ErrConflicts)
	}
	return results, nil
}

// generateChapter renders one chapter to LaTeX and returns the bytes. It
// writes to a temporary file rather than into tex/ so the author's copy is
// never touched before reconcile decides what should land there.
func generateChapter(root string, config Config, chapter string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "paperkit-gen-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, TexName(chapter))
	args := []string{
		"--natbib",
		"--from", "markdown",
		"--to", "latex",
		"--wrap=preserve",
	}
	args = append(args, config.filterArgs()...)
	args = append(args, chapter, "-o", output)
	if err := runIn(root, "pandoc", args...); err != nil {
		return nil, fmt.Errorf("pandoc: %w", err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		return nil, fmt.Errorf("pandoc produced no output: %w", err)
	}
	return generated, nil
}

// generateSkeleton produces tex/main.tex: pandoc's standalone preamble with
// the chapter inputs spliced in place of the body.
//
// The skeleton is generated from the real chapters, not from an empty
// document, because pandoc emits its preamble conditionally on what the
// content uses: a chapter containing a table needs \usepackage{longtable}, one
// containing an image needs graphicx, and pandoc only adds each when it sees
// the feature. A skeleton built from an empty file compiles on its own and
// then fails on the first chapter that uses a table.
func generateSkeleton(root string, config Config) error {
	args := []string{
		"-s", "--natbib",
		"--from", "markdown",
		// Explicit because the skeleton is read from stdout, where pandoc has
		// no output filename to infer the format from.
		"--to", "latex",
		// Relative to the paper directory, which is the command's working
		// directory below; joining it with root as well would resolve it
		// inside itself.
		"--include-in-header=" + config.Preamble,
		"-V", "documentclass=" + config.DocumentClass,
		"-V", "classoption=" + config.ClassOptions,
		"-V", "natbiboptions=numbers,sort&compress",
		"-M", "bibliography=references",
	}
	args = append(args, config.filterArgs()...)
	args = append(args, config.Chapters...)
	args = append(args, "-o", "-")

	command := exec.Command("pandoc", args...)
	command.Dir = root
	skeleton, err := command.Output()
	if err != nil {
		return fmt.Errorf("pandoc skeleton: %w", err)
	}

	body, err := documentBody(root, config)
	if err != nil {
		return err
	}
	document, err := spliceBody(string(skeleton), body)
	if err != nil {
		return err
	}
	// pandoc hard-codes plainnat; the IEEE natbib style is IEEEtranN.
	document = strings.Replace(document,
		`\bibliographystyle{plainnat}`, `\bibliographystyle{IEEEtranN}`, 1)
	document, err = setGraphicsPath(document, config)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, config.TexDir, "main.tex"), []byte(document), 0o644)
}

// setGraphicsPath points graphicx at the paper directory and its figure
// directory.
//
// Figure paths in the chapters are written relative to the paper directory
// (fig/x.pdf, images/y.png) because that is where the markdown lives, but
// latexmk runs in tex/ so those paths resolve a level too low. The declaration
// goes in only when pandoc actually loaded graphicx, since \graphicspath is
// undefined without it and a chapter set with no figures would stop compiling.
func setGraphicsPath(document string, config Config) (string, error) {
	if !strings.Contains(document, "graphicx") {
		return document, nil
	}
	toRoot, err := filepath.Rel(config.TexDir, ".")
	if err != nil {
		return "", err
	}
	toRoot = filepath.ToSlash(toRoot) + "/"
	declaration := fmt.Sprintf("\\graphicspath{{%s}{%s}}\n",
		toRoot, toRoot+filepath.ToSlash(config.FigureDir)+"/")

	index := strings.Index(document, beginDocument)
	if index < 0 {
		return "", fmt.Errorf("pandoc skeleton has no %s", beginDocument)
	}
	return document[:index] + declaration + document[index:], nil
}

// documentBody assembles everything between \begin{document} and the
// bibliography: the title block and abstract, the chapter inputs, and the
// acknowledgment. The title page and acknowledgments are optional, so a paper
// that configures neither gets exactly the chapter inputs.
func documentBody(root string, config Config) (string, error) {
	var body strings.Builder

	if config.TitlePage != "" {
		page, err := ReadTitlePage(filepath.Join(root, config.TitlePage))
		if err != nil {
			return "", err
		}
		body.WriteString(page.titleBlock())
		abstract, err := page.abstractBlock()
		if err != nil {
			return "", err
		}
		if abstract != "" {
			body.WriteString("\n" + abstract)
		}
		extra, err := page.bodyBlock()
		if err != nil {
			return "", err
		}
		if extra != "" {
			body.WriteString("\n" + extra)
		}
		body.WriteString("\n")
	}

	inputs, err := chapterInputs(config)
	if err != nil {
		return "", err
	}
	body.WriteString(inputs)

	if config.Acknowledgments != "" {
		acknowledgment, err := acknowledgmentBlock(filepath.Join(root, config.Acknowledgments))
		if err != nil {
			return "", err
		}
		body.WriteString("\n" + acknowledgment)
	}
	return body.String(), nil
}

func chapterInputs(config Config) (string, error) {
	var builder strings.Builder
	for _, chapter := range config.Chapters {
		name := TexName(chapter)
		builder.WriteString(`\input{` + strings.TrimSuffix(name, ".tex") + "}\n")
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("no chapters to input")
	}
	return builder.String(), nil
}

// markdownToTex converts a fragment of markdown, used for the abstract and the
// acknowledgment, which are prose rather than metadata.
func markdownToTex(markdown []byte) ([]byte, error) {
	command := exec.Command("pandoc",
		"--natbib",
		"--from", "markdown",
		"--to", "latex",
		"--wrap=preserve",
	)
	command.Stdin = bytes.NewReader(markdown)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("pandoc: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// spliceBody replaces the empty body of pandoc's skeleton with the chapter
// inputs, leaving everything else — preamble, bibliography, end matter —
// exactly as pandoc emitted it.
func spliceBody(skeleton, body string) (string, error) {
	start := strings.Index(skeleton, beginDocument)
	if start < 0 {
		return "", fmt.Errorf("pandoc skeleton has no %s", beginDocument)
	}
	start += len(beginDocument)
	end := strings.Index(skeleton[start:], bibliographyLine)
	if end < 0 {
		return "", fmt.Errorf("pandoc skeleton has no %s", bibliographyLine)
	}
	end += start
	return skeleton[:start] + "\n\n" + body + "\n" + skeleton[end:], nil
}

func runIn(dir, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
