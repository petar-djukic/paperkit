// Package paperkit builds the repository's papers: markdown chapters are the
// source of truth, LaTeX under tex/ is a hand-editable intermediate, and
// latexmk produces the PDF. Every paper directory drives the same targets
// through a thin magefile plus a paper.yaml.
//
// The forward path converts through github.com/petar-djukic/md-to-tex and
// assembles the fragments into a container document. It is the only engine:
// the pandoc path it replaced was removed once every paper had converted
// (GH-588). The backport direction still shells out to pandoc, reading LaTeX
// back into markdown, which md-to-tex does not do.
//
// A paper loads natbib in its own preamble. The container names the preamble
// and never generates its contents, so nothing else brings it in, and without
// it the IEEE bibliography style prints each entry's author-year label where
// its number belongs.
package paperkit

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is a paper directory's build description, read from paper.yaml.
type Config struct {
	// Title names the paper in build output. The typeset title comes from the
	// title page, not from here.
	Title string `yaml:"title"`

	// TitlePage is the markdown carrying the paper's title, author, and
	// abstract — metadata in its frontmatter, abstract in its body. Optional:
	// a paper that sets none builds with no title block, which is how the
	// papers that have not adopted one yet keep working.
	TitlePage string `yaml:"title_page"`

	// Acknowledgments is markdown rendered as an unnumbered section before the
	// bibliography. Optional.
	Acknowledgments string `yaml:"acknowledgments"`

	// Chapters lists the markdown sources in reading order. Order here is
	// authoritative: the generated main.tex inputs them exactly as listed,
	// so renumbering a file does not silently reorder the paper.
	Chapters []string `yaml:"chapters"`

	// Preamble is the venue's LaTeX header, included by the generated
	// skeleton. Papers differ here (IEEE two-column against the COMST
	// tutorial layout), so it stays per-paper rather than shared.
	Preamble string `yaml:"preamble"`

	// FigureDir holds the d2 sources and their compiled PDFs.
	FigureDir string `yaml:"figure_dir"`

	// TexDir holds the generated, hand-editable LaTeX and its baseline
	// snapshots.
	TexDir string `yaml:"tex_dir"`

	// BuildDir holds latexmk's working files and the finished PDF.
	BuildDir string `yaml:"build_dir"`

	// Output is the PDF basename written into BuildDir.
	Output string `yaml:"output"`

	// DocumentClass and ClassOptions set the LaTeX class of the generated
	// skeleton. IEEE venues want IEEEtran, whose two-column layout the papers
	// are written against.
	DocumentClass string `yaml:"document_class"`
	ClassOptions  string `yaml:"class_options"`

	// Bibliography is the shared CSL-YAML corpus, relative to the paper
	// directory. Every paper cites from the one database at the repository
	// root rather than keeping a private copy.
	Bibliography string `yaml:"bibliography"`

	// RefsTool converts that corpus to BibTeX, relative to the paper
	// directory.
	RefsTool string `yaml:"refs_tool"`

	// Engine names what converts markdown to LaTeX on the forward path.
	// md-to-tex is the only engine; the field stays so a future engine can be
	// named, and so a paper may state its engine explicitly.
	Engine string `yaml:"engine"`
}

// EngineLibrary converts through md-to-tex. It is the only engine: the pandoc
// path it replaced was removed once every paper had converted (GH-588).
const EngineLibrary = "md-to-tex"

// LoadConfig reads paper.yaml from root and applies defaults. A config naming
// a chapter that is not on disk is an error: a paper that silently drops a
// chapter still builds, and the missing section is easy to miss in a 50-page
// PDF.
func LoadConfig(root string) (Config, error) {
	var config Config
	path := filepath.Join(root, "paper.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("read paper config: %w", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parse %s: %w", path, err)
	}
	config.applyDefaults()
	if err := config.validate(root); err != nil {
		return config, err
	}
	return config, nil
}

func (c *Config) applyDefaults() {
	if c.FigureDir == "" {
		c.FigureDir = "fig"
	}
	if c.TexDir == "" {
		c.TexDir = "tex"
	}
	if c.BuildDir == "" {
		c.BuildDir = "build"
	}
	if c.Output == "" {
		c.Output = "paper.pdf"
	}
	if c.Preamble == "" {
		c.Preamble = filepath.Join("templates", "ieee-preamble.tex")
	}
	if c.DocumentClass == "" {
		c.DocumentClass = "IEEEtran"
	}
	if c.ClassOptions == "" {
		c.ClassOptions = "journal"
	}
	if c.Bibliography == "" {
		c.Bibliography = filepath.Join("..", "references.yaml")
	}
	if c.RefsTool == "" {
		c.RefsTool = filepath.Join("..", "cmd", "refs2bib")
	}
	if c.Engine == "" {
		c.Engine = EngineLibrary
	}
}

func (c Config) validate(root string) error {
	if c.Engine != EngineLibrary {
		return fmt.Errorf("engine %q: %s is the only engine (the pandoc path was removed, GH-588)", c.Engine, EngineLibrary)
	}
	if len(c.Chapters) == 0 {
		return fmt.Errorf("paper config lists no chapters")
	}
	seen := make(map[string]bool, len(c.Chapters))
	for _, chapter := range c.Chapters {
		if seen[chapter] {
			return fmt.Errorf("chapter %s is listed twice", chapter)
		}
		seen[chapter] = true
		if _, err := os.Stat(filepath.Join(root, chapter)); err != nil {
			return fmt.Errorf("chapter %s: %w", chapter, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, c.Preamble)); err != nil {
		return fmt.Errorf("preamble %s: %w", c.Preamble, err)
	}
	for label, optional := range map[string]string{
		"title page":      c.TitlePage,
		"acknowledgments": c.Acknowledgments,
	} {
		if optional == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, optional)); err != nil {
			return fmt.Errorf("%s %s: %w", label, optional, err)
		}
	}
	return nil
}

// roster is every markdown source that becomes a fragment the container
// inputs, in the order it is typeset: the title page, the chapters, then the
// acknowledgment. The optional two are absent from papers that configure
// neither, which is why this is not simply Chapters.
func (c Config) roster() []string {
	roster := make([]string, 0, len(c.Chapters)+2)
	if c.TitlePage != "" {
		roster = append(roster, c.TitlePage)
	}
	roster = append(roster, c.Chapters...)
	if c.Acknowledgments != "" {
		roster = append(roster, c.Acknowledgments)
	}
	return roster
}

// TexName maps a chapter's markdown filename to its generated LaTeX
// filename, keeping the stem so the two are obvious to line up by eye.
func TexName(chapter string) string {
	base := filepath.Base(chapter)
	return base[:len(base)-len(filepath.Ext(base))] + ".tex"
}
