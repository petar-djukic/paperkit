package paperkit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Figures compiles every d2 source in the paper's figure directory to PDF,
// skipping any whose output is already current. Vector output because the
// figures land in LaTeX; stale-only because d2 is slow enough that rebuilding
// all of them on every run is the difference between a usable edit loop and an
// annoying one.
func Figures(root string, config Config) error {
	sources, err := filepath.Glob(filepath.Join(root, config.FigureDir, "*.d2"))
	if err != nil {
		return err
	}
	sort.Strings(sources)
	for _, source := range sources {
		output := strings.TrimSuffix(source, ".d2") + ".pdf"
		if !outOfDate(source, output) {
			continue
		}
		fmt.Printf("figures: d2 %s\n", filepath.Base(source))
		if err := run("d2", source, output); err != nil {
			return fmt.Errorf("d2 %s: %w", filepath.Base(source), err)
		}
	}
	return nil
}

// Bib regenerates the paper's references.bib from the shared CSL-YAML corpus
// through cmd/refs2bib, which already handles the corpus's date shorthand and
// mononym authors. It writes next to main.tex so bibtex finds it without a
// search path.
func Bib(root string, config Config) error {
	texDir := filepath.Join(root, config.TexDir)
	if err := os.MkdirAll(texDir, 0o755); err != nil {
		return err
	}
	output := filepath.Join(config.TexDir, "references.bib")
	fmt.Printf("bib: %s -> %s\n", config.Bibliography, output)
	return runIn(root, "go", "run", config.RefsTool,
		"--yaml", config.Bibliography,
		"--out", output)
}

// PDF compiles tex/main.tex with latexmk.
//
// xelatex rather than pdflatex: corpus titles carry arbitrary Unicode — Greek,
// math, accented names — that only a Unicode-aware engine renders without
// per-character setup. latexmk runs with the tex directory as its working
// directory so \input and the bibliography resolve as written, and drops its
// aux files into the build directory so tex/ holds sources only.
func PDF(root string, config Config) error {
	buildDir := filepath.Join(root, config.BuildDir)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	outDir, err := filepath.Rel(filepath.Join(root, config.TexDir), buildDir)
	if err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fmt.Printf("pdf: latexmk -xelatex -> %s\n", filepath.Join(config.BuildDir, config.Output))
	// TEXINPUTS is what makes the figures resolvable. The chapters reference
	// them relative to the paper directory, latexmk runs in tex/, and with
	// -outdir a parent-relative \graphicspath stops resolving — the aux files
	// have to land outside tex/, which holds committed sources, so the search
	// path moves instead of the output.
	command := exec.Command("latexmk",
		"-xelatex", "-interaction=nonstopmode", "-halt-on-error",
		"-outdir="+outDir, "main.tex")
	command.Dir = filepath.Join(root, config.TexDir)
	command.Env = append(os.Environ(), "TEXINPUTS="+absoluteRoot+"//:")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("latexmk: %w%s", err, logTail(root, config))
	}
	if err := CheckMissingChars(filepath.Join(buildDir, "main.log")); err != nil {
		return err
	}
	if config.Output != "" && config.Output != "main.pdf" {
		return os.Rename(filepath.Join(buildDir, "main.pdf"), filepath.Join(buildDir, config.Output))
	}
	return nil
}

// missingChar matches latexmk's report of a character absent from the fonts,
// e.g. `Missing character: There is no ^^^^202f (U+202F) in font ...`.
var missingChar = regexp.MustCompile(`Missing character: There is no .*?\(U\+([0-9A-Fa-f]{4,6})\)`)

// CheckMissingChars fails when latexmk reported characters the fonts could not
// render. latexmk exits zero on those warnings, so the glyph is dropped from
// the PDF and every gate stays green — which is how 16 U+202F narrow no-break
// spaces survived weeks of clean builds before GH-376 caught them by reading
// the log. A missing glyph is silent corruption of the typeset output, so it
// fails the build.
func CheckMissingChars(logPath string) error {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil // no log to inspect; latexmk's own exit status stands
	}
	counts := map[string]int{}
	for _, match := range missingChar.FindAllStringSubmatch(string(data), -1) {
		counts["U+"+strings.ToUpper(match[1])]++
	}
	if len(counts) == 0 {
		return nil
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s: %s appears %d time(s) but no font provides it; it is dropped from the PDF\n",
			filepath.Base(logPath), name, counts[name])
	}
	return fmt.Errorf("%s: %d character(s) missing from the fonts — map them in the preamble",
		filepath.Base(logPath), len(names))
}

var (
	graphicPattern = regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
	logIssue       = regexp.MustCompile(`(?m)(Undefined control sequence|Citation .* undefined|There were undefined references|Runaway argument|LaTeX Error)`)
)

// Audit reports what would make the paper fail to build or build wrong: a
// figure referenced but never compiled, LaTeX older than the markdown it came
// from, or a compile that succeeds while the log records undefined citations.
//
// Staleness is reported rather than repaired. Tex older than its markdown means
// there are unmerged prose changes upstream, and quietly regenerating them
// underneath the author would merge into their hand edits as a side effect of
// asking a question.
func Audit(root string, config Config) error {
	if err := Figures(root, config); err != nil {
		return err
	}
	problems, err := checkSources(root, config)
	if err != nil {
		return err
	}

	if err := PDF(root, config); err != nil {
		problems = append(problems, fmt.Sprintf("build failed: %v", err))
	}
	if log, err := os.ReadFile(filepath.Join(root, config.BuildDir, "main.log")); err == nil {
		for _, match := range logIssue.FindAllString(string(log), -1) {
			problems = append(problems, "LaTeX log: "+match)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("audit failed:\n  - %s", strings.Join(dedupe(problems), "\n  - "))
	}
	fmt.Println("audit: OK")
	return nil
}

// checkSources reports the problems that are visible without compiling:
// chapters with no LaTeX, LaTeX older than its markdown, and figures a chapter
// references but that were never built.
func checkSources(root string, config Config) ([]string, error) {
	var problems []string

	for _, chapter := range config.Chapters {
		tex := texPath(root, config, chapter)
		texInfo, err := os.Stat(tex)
		if errors.Is(err, os.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("%s has no generated LaTeX (run the tex target)", chapter))
			continue
		}
		if err != nil {
			return nil, err
		}
		if sourceInfo, err := os.Stat(filepath.Join(root, chapter)); err == nil {
			if sourceInfo.ModTime().After(texInfo.ModTime()) {
				problems = append(problems, fmt.Sprintf("%s is newer than its LaTeX (run the tex target)", chapter))
			}
		}
		data, err := os.ReadFile(tex)
		if err != nil {
			return nil, err
		}
		for _, match := range graphicPattern.FindAllStringSubmatch(string(data), -1) {
			if !figureExists(root, config, match[1]) {
				problems = append(problems, fmt.Sprintf("%s: missing figure %q", chapter, match[1]))
			}
		}
	}
	return problems, nil
}

// Clean removes the build directory and every generated figure PDF, leaving
// tex/ alone: it holds the author's hand edits and their baselines, which are
// sources, not artifacts.
func Clean(root string, config Config) error {
	if err := os.RemoveAll(filepath.Join(root, config.BuildDir)); err != nil {
		return err
	}
	pdfs, err := filepath.Glob(filepath.Join(root, config.FigureDir, "*.pdf"))
	if err != nil {
		return err
	}
	for _, pdf := range pdfs {
		if err := os.Remove(pdf); err != nil {
			return err
		}
	}
	return nil
}

// figureExists resolves an \includegraphics target the way graphicx does:
// relative to the LaTeX file's directory and to the graphics path, trying the
// usual extensions when the reference omits one.
func figureExists(root string, config Config, name string) bool {
	candidates := []string{name}
	if filepath.Ext(name) == "" {
		for _, extension := range []string{".pdf", ".png", ".jpg", ".jpeg", ".eps"} {
			candidates = append(candidates, name+extension)
		}
	}
	for _, candidate := range candidates {
		for _, base := range []string{root, filepath.Join(root, config.FigureDir), filepath.Join(root, config.TexDir)} {
			if _, err := os.Stat(filepath.Join(base, candidate)); err == nil {
				return true
			}
		}
	}
	return false
}

func logTail(root string, config Config) string {
	log, err := os.ReadFile(filepath.Join(root, config.BuildDir, "main.log"))
	if err != nil {
		return ""
	}
	matches := logIssue.FindAllString(string(log), -1)
	if len(matches) == 0 {
		return ""
	}
	return "\n  " + strings.Join(dedupe(matches), "\n  ")
}

func outOfDate(source, output string) bool {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false
	}
	outputInfo, err := os.Stat(output)
	if err != nil {
		return true
	}
	return sourceInfo.ModTime().After(outputInfo.ModTime())
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	var unique []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
