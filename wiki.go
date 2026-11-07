package paperkit

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Wiki builds a single HTML page from markdown: pandoc renders each section
// paper.yaml's wiki.sections names, the results fill the {{NAME}} placeholders
// of wiki.template, and every figure the markdown references is inlined, so
// the page carries its pictures with it. Diagrams render first, so the PNGs
// the markdown embeds are current.
func Wiki(root string, config Config) error {
	if config.Wiki.Template == "" || len(config.Wiki.Sections) == 0 {
		return fmt.Errorf("paper.yaml configures no wiki (wiki: {template, sections})")
	}
	if err := Diagrams(root, config); err != nil {
		return err
	}
	template, err := os.ReadFile(filepath.Join(root, config.Wiki.Template))
	if err != nil {
		return err
	}
	page := string(template)

	names := make([]string, 0, len(config.Wiki.Sections))
	for name := range config.Wiki.Sections {
		names = append(names, name)
	}
	sort.Strings(names)
	sources := make([]string, 0, len(names))
	for _, name := range names {
		source := config.Wiki.Sections[name]
		html, err := pandocHTML(root, source)
		if err != nil {
			return err
		}
		if html, err = inlineFigures(root, config, html); err != nil {
			return err
		}
		placeholder := "{{" + name + "}}"
		if !strings.Contains(page, placeholder) {
			return fmt.Errorf("%s has no %s placeholder for %s", config.Wiki.Template, placeholder, source)
		}
		page = strings.Replace(page, placeholder, html, 1)
		sources = append(sources, source)
	}

	output := filepath.Join(root, config.Wiki.Output)
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	fmt.Printf("wiki: pandoc %s -> %s\n", strings.Join(sources, ", "), config.Wiki.Output)
	return os.WriteFile(output, []byte(page), 0o644)
}

func pandocHTML(root, source string) (string, error) {
	command := exec.Command("pandoc", "-f", "gfm", "-t", "html", source)
	command.Dir = root
	command.Stderr = os.Stderr
	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("pandoc %s: %w", source, err)
	}
	return string(out), nil
}

// figureGround is the fixed light background the SVG figures are drawn on so
// they read anywhere; inlined, the page supplies the ground instead.
var figureGround = regexp.MustCompile(`<rect x="0" y="0" width="\d+" height="\d+" fill="#f6f7f5"/>\n?`)

// inlineFigures replaces each image under the diagrams directory with its
// data: a PNG becomes a base64 data URI, and an SVG is inlined with its fixed
// ink and ground swapped for the page's theme tokens, so it reads in light and
// dark mode.
func inlineFigures(root string, config Config, html string) (string, error) {
	dir := config.Diagrams
	if dir == "" {
		dir = "figures"
	}
	prefix := regexp.QuoteMeta(filepath.ToSlash(dir) + "/")
	pngImage := regexp.MustCompile(`<img src="(` + prefix + `[^"]+\.png)"`)
	svgImage := regexp.MustCompile(`<img src="(` + prefix + `[^"]+\.svg)"[^>]*>`)

	var failure error
	html = pngImage.ReplaceAllStringFunc(html, func(attribute string) string {
		data, err := os.ReadFile(filepath.Join(root, pngImage.FindStringSubmatch(attribute)[1]))
		if err != nil {
			failure = err
			return attribute
		}
		return `<img class="diagram" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(data) + `"`
	})
	html = svgImage.ReplaceAllStringFunc(html, func(tag string) string {
		data, err := os.ReadFile(filepath.Join(root, svgImage.FindStringSubmatch(tag)[1]))
		if err != nil {
			failure = err
			return tag
		}
		svg := figureGround.ReplaceAllString(string(data), "")
		svg = strings.ReplaceAll(svg, "#20262c", "currentColor")
		svg = strings.ReplaceAll(svg, "#f6f7f5", "var(--ground)")
		return strings.Replace(svg, "<svg ", `<svg class="fig" `, 1)
	})
	return html, failure
}
