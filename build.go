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

	"github.com/petar-djukic/paperkit/bib"
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

// Diagrams renders every d2 source in the figure directory to a PNG in the
// configured diagrams directory, the form markdown pages embed, so a page read
// on GitHub or in Obsidian shows the same picture the paper compiles to PDF.
// A paper that configures no diagrams directory renders none.
func Diagrams(root string, config Config) error {
	if config.Diagrams == "" {
		return nil
	}
	sources, err := filepath.Glob(filepath.Join(root, config.FigureDir, "*.d2"))
	if err != nil {
		return err
	}
	sort.Strings(sources)
	for _, source := range sources {
		output := filepath.Join(root, config.Diagrams, strings.TrimSuffix(filepath.Base(source), ".d2")+".png")
		if !outOfDate(source, output) {
			continue
		}
		fmt.Printf("diagrams: d2 %s -> %s\n", filepath.Base(source), filepath.Join(config.Diagrams, filepath.Base(output)))
		if err := run("d2", source, output); err != nil {
			return fmt.Errorf("d2 %s: %w", filepath.Base(source), err)
		}
		// d2 renames an owner-only temp file into place; the PNG is read by
		// editors and the wiki, so it gets ordinary file permissions.
		if err := os.Chmod(output, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Bib regenerates the paper's references.bib from its CSL-YAML corpus. It
// writes next to main.tex so bibtex finds it without a search path.
func Bib(root string, config Config) error {
	output := filepath.Join(config.TexDir, "references.bib")
	fmt.Printf("bib: %s -> %s\n", config.Bibliography, output)
	_, err := bib.Convert(filepath.Join(root, config.Bibliography), filepath.Join(root, output))
	return err
}

// PDF compiles tex/main.tex with latexmk.
//
// xelatex rather than pdflatex: corpus titles carry arbitrary Unicode — Greek,
// math, accented names — that only a Unicode-aware engine renders without
// per-character setup. latexmk runs with the tex directory as its working
// directory so \input and the bibliography resolve as written, and drops its
// aux files into the build directory so tex/ holds only generated LaTeX.
func PDF(root string, config Config) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fmt.Printf("pdf: latexmk -xelatex -> %s\n", filepath.Join(config.BuildDir, config.Output))
	// TEXINPUTS is what makes the figures resolvable. The chapters reference
	// them relative to the paper directory, latexmk runs in tex/, and with
	// -outdir a parent-relative \graphicspath stops resolving, so the search
	// path moves instead of the output.
	return latexmk(filepath.Join(root, config.TexDir), filepath.Join(root, config.BuildDir),
		"main.tex", "main", config.Output, absoluteRoot+"//:")
}

// latexmk compiles source, a file in texDir, with xelatex under jobname,
// landing the aux files and the PDF in buildDir, and names the PDF output.
// texinputs is the TeX search path.
//
// The paper compiles with a recursive search of its whole directory, which
// reaches every build output under it. Each document a paper builds therefore
// takes its own job name: under a shared one, the paper's first pass finds the
// outline's main.bbl and adopts it as its own bibliography. For the same
// reason the source is named relative to texDir, not as a bare name another
// tex directory also holds.
func latexmk(texDir, buildDir, source, jobname, output, texinputs string) error {
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	outDir, err := filepath.Rel(texDir, buildDir)
	if err != nil {
		return err
	}
	command := exec.Command("latexmk",
		"-xelatex", "-interaction=nonstopmode", "-halt-on-error",
		"-outdir="+outDir, "-jobname="+jobname, "./"+source)
	command.Dir = texDir
	command.Env = append(os.Environ(), "TEXINPUTS="+texinputs)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	logPath := filepath.Join(buildDir, jobname+".log")
	if err := command.Run(); err != nil {
		return fmt.Errorf("latexmk: %w%s", err, logTail(logPath))
	}
	if err := CheckMissingChars(logPath); err != nil {
		return err
	}
	if output != "" && output != jobname+".pdf" {
		return os.Rename(filepath.Join(buildDir, jobname+".pdf"), filepath.Join(buildDir, output))
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
// Staleness is reported rather than repaired, so asking the question never
// changes the build: the tex target regenerates.
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

	// Drift is reported, not failed: the corpus changes whenever the reference
	// pipeline runs, and which side of a drifted entry is right is the
	// author's call (see ReferenceDrift).
	drift, err := ReferenceDrift(root, config)
	if err != nil {
		return err
	}
	if len(drift) > 0 {
		fmt.Printf("audit: %d reference(s) differ from %s: %s\n",
			len(drift), config.Corpus, strings.Join(drift, ", "))
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

// Clean removes everything the build generates: the build directory, the tex
// directory, and every compiled figure PDF. Removing tex/ also drops fragments
// left behind by a chapter that was renamed or taken off the roster.
func Clean(root string, config Config) error {
	for _, dir := range []string{config.BuildDir, config.TexDir} {
		if err := os.RemoveAll(filepath.Join(root, dir)); err != nil {
			return err
		}
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

func logTail(logPath string) string {
	log, err := os.ReadFile(logPath)
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
