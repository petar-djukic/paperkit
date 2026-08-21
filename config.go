// Package paperkit builds the repository's papers: markdown chapters are the
// source of truth, LaTeX under tex/ is a hand-editable intermediate, and
// latexmk produces the PDF. Every paper directory drives the same targets
// through a thin magefile plus a paper.yaml.
//
// # Two forward engines
//
// A paper names its engine in paper.yaml. The default, pandoc, converts each
// chapter through pandoc and the Lua filters the paper configures. The other,
// md-to-tex, converts through github.com/petar-djukic/md-to-tex: no filters,
// no pandoc process, and a container document in place of pandoc's harvested
// skeleton. The backport direction and the PDF compile are the same either
// way, because the library does neither.
//
// autonomous-network-tutorial converts through the library (GH-468). The
// other four papers stay on pandoc, and the reason is their markdown rather
// than the engine: the library requires a caption where pandoc lets an
// uncaptioned table through, and captions are written once, in the markdown,
// so a table without one has nowhere to keep it. Each paper moves when its
// own migration issue is done — GH-473, GH-474, GH-475, which carry the
// per-chapter counts.
//
// A paper on the library path also loads natbib in its own preamble. The
// container names the preamble and never generates its contents, so nothing
// else brings it in, and without it the IEEE bibliography style prints each
// entry's author-year label where its number belongs.
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

	// Filters are pandoc Lua filters applied to every chapter and to the
	// skeleton, relative to the paper directory.
	//
	// A two-column class needs them. Pandoc renders a markdown table as a
	// longtable, and longtable refuses to run in two-column mode, so an
	// IEEEtran paper with any table fails to compile until a filter rewrites
	// those tables into table* floats.
	Filters []string `yaml:"filters"`

	// Engine names what converts markdown to LaTeX on the forward path.
	//
	// The papers do not all convert the same way yet. md-to-tex requires
	// captions where pandoc invents them — a table states its caption in a
	// Table: line, a figure in its alt text — so a paper moves over once its
	// markdown carries them. Until then it stays on pandoc with the filters,
	// and both paths are maintained rather than one being a migration
	// staging area.
	Engine string `yaml:"engine"`
}

// The engines a paper may name.
const (
	// EnginePandoc converts through pandoc and the configured Lua filters.
	EnginePandoc = "pandoc"
	// EngineLibrary converts through md-to-tex, which needs no filters and no
	// pandoc process on the forward path.
	EngineLibrary = "md-to-tex"
)

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
		c.Engine = EnginePandoc
	}
}

func (c Config) validate(root string) error {
	if c.Engine != EnginePandoc && c.Engine != EngineLibrary {
		return fmt.Errorf("engine %q is neither %s nor %s", c.Engine, EnginePandoc, EngineLibrary)
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
	for _, filter := range c.Filters {
		if _, err := os.Stat(filepath.Join(root, filter)); err != nil {
			return fmt.Errorf("filter %s: %w", filter, err)
		}
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
//
// The pandoc path has no equivalent: it splices the title block into main.tex
// rather than giving it a fragment of its own.
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

// filterArgs renders the configured filters as pandoc arguments. The paths stay
// relative because pandoc runs with the paper directory as its working
// directory.
func (c Config) filterArgs() []string {
	args := make([]string, 0, len(c.Filters)*2)
	for _, filter := range c.Filters {
		args = append(args, "--lua-filter="+filter)
	}
	return args
}

// TexName maps a chapter's markdown filename to its generated LaTeX
// filename, keeping the stem so the two are obvious to line up by eye.
func TexName(chapter string) string {
	base := filepath.Base(chapter)
	return base[:len(base)-len(filepath.Ext(base))] + ".tex"
}
