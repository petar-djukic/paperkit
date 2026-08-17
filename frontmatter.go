package paperkit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// TitlePage is what the paper's title-page markdown carries: metadata in its
// YAML frontmatter, and an abstract in one of two places.
//
// The papers write it differently. One states the abstract as a frontmatter
// field, which is pandoc's own convention; the other marks it in the body with
// a bold-italic run, so the retired build would not number it as a section.
// Both are read, the frontmatter field winning when a page has both.
type TitlePage struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
	Date     string `yaml:"date"`
	Author   string `yaml:"author"`

	// Abstract is the frontmatter field when the page states one, otherwise
	// the body below the abstract marker.
	Abstract string `yaml:"abstract"`

	// Body is what remains below the frontmatter once the abstract is
	// accounted for. A page whose abstract is a frontmatter field can still
	// carry front matter in its body — an IEEEkeywords block, for instance —
	// which is emitted after the abstract.
	Body string `yaml:"-"`
}

var (
	frontmatterBlock = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n`)
	// The title page marks its abstract with a bold-italic run rather than a
	// heading, so it does not become a numbered section in the older build.
	abstractMarker = regexp.MustCompile(`(?m)^\s*\*{2,3}Abstract\*{2,3}\s*$`)
	// The author is stored as markdown, name followed by a mailto link.
	authorEmail = regexp.MustCompile(`\[([^\]]+)\]\(mailto:[^)]+\)`)
)

// ReadTitlePage parses the paper's title-page markdown.
//
// It is parsed rather than run through the chapter path because its parts have
// specific destinations: the metadata belongs in \title and \author, and the
// abstract belongs in IEEEtran's abstract environment. Treated as a chapter it
// would come out as body text under a section heading.
func ReadTitlePage(path string) (TitlePage, error) {
	var page TitlePage
	data, err := os.ReadFile(path)
	if err != nil {
		return page, fmt.Errorf("read title page: %w", err)
	}

	match := frontmatterBlock.FindSubmatch(data)
	if match == nil {
		return page, fmt.Errorf("%s has no YAML frontmatter", filepath.Base(path))
	}
	if err := yaml.Unmarshal(match[1], &page); err != nil {
		return page, fmt.Errorf("%s frontmatter: %w", filepath.Base(path), err)
	}
	if strings.TrimSpace(page.Title) == "" {
		return page, fmt.Errorf("%s frontmatter has no title", filepath.Base(path))
	}

	body := data[len(match[0]):]
	if strings.TrimSpace(page.Abstract) != "" {
		// The abstract came from the frontmatter, so whatever is in the body
		// is something else and belongs after it.
		page.Body = strings.TrimSpace(string(body))
		return page, nil
	}
	if marker := abstractMarker.FindIndex(body); marker != nil {
		body = body[marker[1]:]
	}
	page.Abstract = strings.TrimSpace(string(body))
	return page, nil
}

// titleBlock renders the IEEEtran title, author, and abstract.
//
// The subtitle joins the title rather than taking a command of its own:
// IEEEtran has no \subtitle, and a \thanks or second \title line would either
// be dropped or land in the author block.
func (p TitlePage) titleBlock() string {
	var block strings.Builder

	title := escapeLaTeX(p.Title)
	if subtitle := strings.TrimSpace(p.Subtitle); subtitle != "" {
		title += `\\ \large ` + escapeLaTeX(subtitle)
	}
	block.WriteString("\\title{" + title + "}\n")

	if author := p.authorField(); author != "" {
		block.WriteString("\\author{" + author + "}\n")
	}
	block.WriteString("\\maketitle\n")
	return block.String()
}

// authorField turns the markdown author line into IEEEtran's author field,
// rendering the mailto link as a plain address rather than passing the
// markdown through verbatim.
func (p TitlePage) authorField() string {
	author := strings.TrimSpace(p.Author)
	if author == "" {
		return ""
	}
	var email string
	if match := authorEmail.FindStringSubmatch(author); match != nil {
		email = match[1]
		author = strings.TrimSpace(authorEmail.ReplaceAllString(author, ""))
	}
	field := escapeLaTeX(author)
	if email != "" {
		field += `\\ \texttt{` + escapeLaTeX(email) + `}`
	}
	return field
}

// abstractBlock renders the abstract environment. The abstract is prose from
// the markdown, so it is converted by pandoc rather than escaped: it contains
// the paper's own emphasis and terminology.
func (p TitlePage) abstractBlock() (string, error) {
	if strings.TrimSpace(p.Abstract) == "" {
		return "", nil
	}
	rendered, err := markdownToTex([]byte(p.Abstract))
	if err != nil {
		return "", fmt.Errorf("render abstract: %w", err)
	}
	return "\\begin{abstract}\n" + strings.TrimSpace(string(rendered)) + "\n\\end{abstract}\n", nil
}

// bodyBlock renders whatever the title page carries below its frontmatter when
// the abstract did not come from there — a keywords block, typically. It is
// converted rather than escaped so raw-LaTeX passthrough survives.
func (p TitlePage) bodyBlock() (string, error) {
	if strings.TrimSpace(p.Body) == "" {
		return "", nil
	}
	rendered, err := markdownToTex([]byte(p.Body))
	if err != nil {
		return "", fmt.Errorf("render title page body: %w", err)
	}
	return strings.TrimSpace(string(rendered)) + "\n", nil
}

// acknowledgmentBlock renders the acknowledgments markdown as an unnumbered
// section. Its own heading is dropped: IEEEtran papers name this section by
// convention, and a converted "# Acknowledgment" would print the title twice.
func acknowledgmentBlock(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read acknowledgments: %w", err)
	}
	if match := frontmatterBlock.FindSubmatch(data); match != nil {
		data = data[len(match[0]):]
	}
	body := regexp.MustCompile(`(?m)\A\s*#\s+.*\r?\n`).ReplaceAll(bytes.TrimSpace(data), nil)

	rendered, err := markdownToTex(bytes.TrimSpace(body))
	if err != nil {
		return "", fmt.Errorf("render acknowledgments: %w", err)
	}
	return "\\section*{Acknowledgment}\n" + strings.TrimSpace(string(rendered)) + "\n", nil
}

// escapeLaTeX protects the characters that would otherwise be markup in a
// title or author field.
func escapeLaTeX(text string) string {
	replacer := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`&`, `\&`,
		`%`, `\%`,
		`$`, `\$`,
		`#`, `\#`,
		`_`, `\_`,
		`{`, `\{`,
		`}`, `\}`,
		`~`, `\textasciitilde{}`,
		`^`, `\textasciicircum{}`,
	)
	return replacer.Replace(text)
}
