package paperkit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/petar-djukic/paperkit/srd"
)

// Outline renders the paper's outline from docs/srd/ and compiles it to
// build/outline.pdf, in the form paper.yaml's outline.format names.
//
// The outline is generated from the requirements rather than the prose, so it
// shows what the paper is committed to saying before the sections exist.
func Outline(root string, config Config) error {
	switch srd.Format(config.Outline.Format) {
	case srd.TeX:
		return outlineTeX(root, config)
	case srd.Markdown:
		return outlineMarkdown(root, config)
	case "":
		return fmt.Errorf("paper.yaml sets no outline format (outline: {format: tex|md})")
	default:
		return fmt.Errorf("outline format %q: want tex or md", config.Outline.Format)
	}
}

// outlineTeX writes outline.tex, a standalone IEEEtran document, and compiles
// it in the paper's tex directory, so it resolves the paper's bibliography and
// figure search path.
func outlineTeX(root string, config Config) error {
	if err := Figures(root, config); err != nil {
		return err
	}
	if err := Bib(root, config); err != nil {
		return err
	}
	fmt.Println("outline: docs/srd -> outline.tex")
	generated, err := srd.Render(filepath.Join(root, "docs"), srd.TeX)
	if err != nil {
		return err
	}
	generated = dropEmptyBibliography(generated)
	texDir := filepath.Join(root, config.TexDir)
	if err := os.MkdirAll(texDir, 0o755); err != nil {
		return err
	}
	for _, path := range []string{filepath.Join(root, "outline.tex"), filepath.Join(texDir, "outline.tex")} {
		if err := os.WriteFile(path, generated, 0o644); err != nil {
			return err
		}
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fmt.Printf("outline: latexmk -xelatex -> %s\n", filepath.Join(config.BuildDir, "outline.pdf"))
	return latexmk(texDir, filepath.Join(root, config.BuildDir), "outline.tex", "outline.pdf", absoluteRoot+"//:")
}

// outlineMarkdown writes outline.md and compiles it as a one-chapter paper
// with its own tex directory, sharing the paper's title page, class, preamble
// and bibliography, so the markdown stays readable in any editor and the PDF
// typesets like a chapter.
func outlineMarkdown(root string, config Config) error {
	fmt.Println("outline: docs/srd -> outline.md")
	generated, err := srd.Render(filepath.Join(root, "docs"), srd.Markdown)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "outline.md"), generated, 0o644); err != nil {
		return err
	}

	outline := config
	outline.Chapters = []string{"outline.md"}
	outline.TexDir = "tex-outline"
	outline.BuildDir = filepath.Join(config.BuildDir, "outline")
	outline.Output = "outline.pdf"
	if config.Outline.ClassOptions != "" {
		outline.ClassOptions = config.Outline.ClassOptions
	}
	if err := Figures(root, outline); err != nil {
		return err
	}
	if err := Bib(root, outline); err != nil {
		return err
	}
	if err := GenerateTex(root, outline); err != nil {
		return err
	}
	// A recursive search path from the paper root, as PDF uses, would let the
	// outline find the paper's own main.tex and main.bbl. Naming only the
	// template directories still finds a venue class kept under templates/.
	texinputs, err := templateSearchPath(root)
	if err != nil {
		return err
	}
	fmt.Printf("outline: latexmk -xelatex -> %s\n", filepath.Join(outline.BuildDir, outline.Output))
	return latexmk(filepath.Join(root, outline.TexDir), filepath.Join(root, outline.BuildDir),
		"main.tex", outline.Output, texinputs)
}

// dropEmptyBibliography removes the bibliography block from an outline that
// cites nothing: bibtex would emit an empty thebibliography, which LaTeX
// rejects as a missing \item.
func dropEmptyBibliography(tex []byte) []byte {
	if bytes.Contains(tex, []byte(`\cite{`)) {
		return tex
	}
	tex = bytes.ReplaceAll(tex, []byte("\\bibliographystyle{IEEEtran}\n"), nil)
	return bytes.ReplaceAll(tex, []byte("\\bibliography{references}\n"), nil)
}

// templateSearchPath lists templates/ and each directory directly under it,
// joined for TEXINPUTS with a trailing separator so TeX also searches its
// default path.
func templateSearchPath(root string) (string, error) {
	templates, err := filepath.Abs(filepath.Join(root, "templates"))
	if err != nil {
		return "", err
	}
	dirs := []string{templates}
	entries, err := os.ReadDir(templates)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(templates, entry.Name()))
		}
	}
	separator := string(os.PathListSeparator)
	return strings.Join(dirs, separator) + separator, nil
}
