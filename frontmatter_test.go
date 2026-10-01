package paperkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const titlePageSource = `---
title: A Reference Architecture for L5 Autonomous Networks
subtitle: "Level 5 Autonomy: What It Would Take"
date: August 2026
author: Petar Djukic [petar.djukic@example.com](mailto:petar.djukic@example.com)
tags:
  - architecture
---

***Abstract***

This document presents a reference architecture.

It decomposes the system into two loops.
`

func TestReadTitlePageSplitsMetadataFromAbstract(t *testing.T) {
	path := writeTemp(t, "00-title-page.md", titlePageSource)

	page, err := ReadTitlePage(path)
	if err != nil {
		t.Fatalf("ReadTitlePage() error: %v", err)
	}
	if page.Title != "A Reference Architecture for L5 Autonomous Networks" {
		t.Errorf("Title = %q", page.Title)
	}
	if page.Subtitle != "Level 5 Autonomy: What It Would Take" {
		t.Errorf("Subtitle = %q", page.Subtitle)
	}
	if page.Date != "August 2026" {
		t.Errorf("Date = %q", page.Date)
	}
	// The abstract marker itself must not survive into the abstract.
	if strings.Contains(page.Abstract, "Abstract") {
		t.Errorf("abstract still carries its marker:\n%s", page.Abstract)
	}
	if !strings.HasPrefix(page.Abstract, "This document presents") {
		t.Errorf("abstract does not start at the body:\n%s", page.Abstract)
	}
	if !strings.Contains(page.Abstract, "two loops") {
		t.Errorf("abstract dropped its later paragraphs:\n%s", page.Abstract)
	}
	// Frontmatter must not leak into the abstract.
	if strings.Contains(page.Abstract, "tags:") {
		t.Errorf("frontmatter leaked into the abstract:\n%s", page.Abstract)
	}
}

// The Proceedings paper states its abstract as a frontmatter field, pandoc's
// own convention, and keeps an IEEEkeywords block in the body. The body must
// not be mistaken for the abstract.
func TestReadTitlePagePrefersTheFrontmatterAbstract(t *testing.T) {
	path := writeTemp(t, "00-title-page.md", `---
title: Autogenic Systems
author: Petar Djukic
abstract: |
  Advances in code-generating AI are moving software toward systems
  that evolve themselves.
---

`+"```{=latex}\n\\begin{IEEEkeywords}\nAutogenic systems, LLM agents.\n\\end{IEEEkeywords}\n```\n")

	page, err := ReadTitlePage(path)
	if err != nil {
		t.Fatalf("ReadTitlePage() error: %v", err)
	}
	if !strings.HasPrefix(page.Abstract, "Advances in code-generating AI") {
		t.Errorf("Abstract did not come from the frontmatter: %q", page.Abstract)
	}
	if strings.Contains(page.Abstract, "IEEEkeywords") {
		t.Errorf("the keywords block was swallowed into the abstract:\n%s", page.Abstract)
	}
	if !strings.Contains(page.Body, "IEEEkeywords") {
		t.Errorf("the keywords block was dropped instead of kept as body:\n%s", page.Body)
	}
}

// The reference-architecture paper marks its abstract in the body instead, and
// has nothing after it.
func TestReadTitlePageFallsBackToTheBodyMarker(t *testing.T) {
	path := writeTemp(t, "00-title-page.md", titlePageSource)

	page, err := ReadTitlePage(path)
	if err != nil {
		t.Fatalf("ReadTitlePage() error: %v", err)
	}
	if !strings.HasPrefix(page.Abstract, "This document presents") {
		t.Errorf("Abstract did not come from the body: %q", page.Abstract)
	}
	if page.Body != "" {
		t.Errorf("body should be empty when the abstract came from it: %q", page.Body)
	}
}

func TestReadTitlePageRejectsMissingFrontmatterAndTitle(t *testing.T) {
	noFrontmatter := writeTemp(t, "a.md", "# Just a heading\n")
	if _, err := ReadTitlePage(noFrontmatter); err == nil {
		t.Error("a title page with no frontmatter was accepted")
	}

	noTitle := writeTemp(t, "b.md", "---\nauthor: Someone\n---\n\nBody.\n")
	if _, err := ReadTitlePage(noTitle); err == nil || !strings.Contains(err.Error(), "no title") {
		t.Errorf("missing-title error = %v", err)
	}

	if _, err := ReadTitlePage(filepath.Join(t.TempDir(), "absent.md")); err == nil {
		t.Error("a missing title page was accepted")
	}
}

func TestTitlePageWithNoAbstractRendersNoAbstractBlock(t *testing.T) {
	requirePandoc(t)
	path := writeTemp(t, "c.md", "---\ntitle: Bare\n---\n")

	page, err := ReadTitlePage(path)
	if err != nil {
		t.Fatal(err)
	}
	abstract, err := page.abstractBlock()
	if err != nil {
		t.Fatal(err)
	}
	if abstract != "" {
		t.Errorf("a title page with no body produced an abstract:\n%s", abstract)
	}
}

func TestAuthorFieldRendersNameAndEmail(t *testing.T) {
	page := TitlePage{Author: "Petar Djukic [petar.djukic@example.com](mailto:petar.djukic@example.com)"}

	field := page.authorField()
	if !strings.Contains(field, "Petar Djukic") {
		t.Errorf("author field lost the name: %q", field)
	}
	if !strings.Contains(field, `\texttt{petar.djukic@example.com}`) {
		t.Errorf("author field did not render the email: %q", field)
	}
	// The markdown link syntax must not reach LaTeX.
	if strings.Contains(field, "](mailto:") || strings.Contains(field, "[petar") {
		t.Errorf("markdown link syntax survived into LaTeX: %q", field)
	}

	if plain := (TitlePage{Author: "Someone Else"}).authorField(); plain != "Someone Else" {
		t.Errorf("a plain author was altered: %q", plain)
	}
	if empty := (TitlePage{}).authorField(); empty != "" {
		t.Errorf("an absent author produced %q", empty)
	}
}

func TestTitleBlockCarriesTitleSubtitleAndMaketitle(t *testing.T) {
	page := TitlePage{Title: "Main Title", Subtitle: "The Subtitle", Author: "Someone"}

	block := page.titleBlock()
	for _, want := range []string{`\title{`, "Main Title", "The Subtitle", `\author{`, `\maketitle`} {
		if !strings.Contains(block, want) {
			t.Errorf("title block is missing %q:\n%s", want, block)
		}
	}

	// IEEEtran has no \subtitle; a paper without one must not emit stray
	// formatting for it.
	bare := TitlePage{Title: "Only Title"}.titleBlock()
	if strings.Contains(bare, `\large`) {
		t.Errorf("a paper with no subtitle emitted subtitle formatting:\n%s", bare)
	}
}

func TestEscapeLaTeXProtectsMarkupCharacters(t *testing.T) {
	for input, want := range map[string]string{
		"Cost & Benefit": `Cost \& Benefit`,
		"100% coverage":  `100\% coverage`,
		"snake_case":     `snake\_case`,
		"a#b":            `a\#b`,
		"plain title":    "plain title",
	} {
		if got := escapeLaTeX(input); got != want {
			t.Errorf("escapeLaTeX(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAcknowledgmentBlockDropsFrontmatterAndItsOwnHeading(t *testing.T) {
	requirePandoc(t)
	path := writeTemp(t, "acknowledgments.md",
		"---\ntags:\n  - governance\n---\n# Acknowledgment\n\nThe author thanks the reviewers.\n")

	block, err := acknowledgmentBlock(path)
	if err != nil {
		t.Fatalf("acknowledgmentBlock() error: %v", err)
	}
	if !strings.HasPrefix(block, `\section*{Acknowledgment}`) {
		t.Errorf("block does not open with the unnumbered section:\n%s", block)
	}
	if !strings.Contains(block, "thanks the reviewers") {
		t.Errorf("block lost its body:\n%s", block)
	}
	// The markdown heading must not print a second time.
	if strings.Count(block, "Acknowledgment") != 1 {
		t.Errorf("the heading was printed twice:\n%s", block)
	}
	if strings.Contains(block, "governance") {
		t.Errorf("frontmatter leaked into the block:\n%s", block)
	}
}

func TestDocumentBodyOrdersTitleChaptersAcknowledgment(t *testing.T) {
	requirePandoc(t)
	root := paperFixture(t, `chapters:
  - 01-first.md
title_page: 00-title-page.md
acknowledgments: acknowledgments.md
`)
	writeFile(t, root, "01-first.md", "# First\n")
	writeFile(t, root, "00-title-page.md", titlePageSource)
	writeFile(t, root, "acknowledgments.md", "# Acknowledgment\n\nThanks.\n")

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	body, err := documentBody(root, config)
	if err != nil {
		t.Fatalf("documentBody() error: %v", err)
	}

	title := strings.Index(body, `\maketitle`)
	abstract := strings.Index(body, `\begin{abstract}`)
	chapter := strings.Index(body, `\input{01-first}`)
	acknowledgment := strings.Index(body, `\section*{Acknowledgment}`)
	for _, part := range []struct {
		name  string
		index int
	}{{"title", title}, {"abstract", abstract}, {"chapter", chapter}, {"acknowledgment", acknowledgment}} {
		if part.index < 0 {
			t.Fatalf("document body is missing the %s:\n%s", part.name, body)
		}
	}
	if !(title < abstract && abstract < chapter && chapter < acknowledgment) {
		t.Errorf("parts are out of order (title %d, abstract %d, chapter %d, ack %d)",
			title, abstract, chapter, acknowledgment)
	}
}

// A paper that configures neither must build exactly as it did before the
// front matter existed.
func TestDocumentBodyWithoutFrontMatterIsChaptersOnly(t *testing.T) {
	root := paperFixture(t, "chapters:\n  - 01-first.md\n", "01-first.md")
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}

	body, err := documentBody(root, config)
	if err != nil {
		t.Fatalf("documentBody() error: %v", err)
	}
	if body != "\\input{01-first}\n" {
		t.Errorf("body = %q, want the chapter input alone", body)
	}
}

func TestLoadConfigRejectsMissingTitlePageAndAcknowledgments(t *testing.T) {
	missingTitle := paperFixture(t,
		"chapters:\n  - 01-intro.md\ntitle_page: 00-absent.md\n", "01-intro.md")
	if _, err := LoadConfig(missingTitle); err == nil || !strings.Contains(err.Error(), "title page") {
		t.Errorf("missing title page error = %v", err)
	}

	missingAck := paperFixture(t,
		"chapters:\n  - 01-intro.md\nacknowledgments: absent.md\n", "01-intro.md")
	if _, err := LoadConfig(missingAck); err == nil || !strings.Contains(err.Error(), "acknowledgments") {
		t.Errorf("missing acknowledgments error = %v", err)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadTitlePageAcceptsAnAuthorList(t *testing.T) {
	// pandoc's own convention writes author as a sequence; the papers differ,
	// so both forms have to reach the same \author field.
	root := t.TempDir()
	writeFile(t, root, "00-title-page.md", strings.Join([]string{
		"---",
		"title: A Tutorial",
		"author:",
		"  - Ada Lovelace",
		"  - Alan Turing",
		"abstract: >",
		"  One paragraph.",
		"---",
		"",
	}, "\n"))

	page, err := ReadTitlePage(filepath.Join(root, "00-title-page.md"))
	if err != nil {
		t.Fatalf("ReadTitlePage() error: %v", err)
	}
	if got, want := string(page.Author), "Ada Lovelace and Alan Turing"; got != want {
		t.Errorf("Author = %q, want %q", got, want)
	}
}

func TestReadTitlePageAcceptsAScalarAuthor(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "00-title-page.md", strings.Join([]string{
		"---",
		"title: A Paper",
		"author: Ada Lovelace",
		"---",
		"",
	}, "\n"))

	page, err := ReadTitlePage(filepath.Join(root, "00-title-page.md"))
	if err != nil {
		t.Fatalf("ReadTitlePage() error: %v", err)
	}
	if got, want := string(page.Author), "Ada Lovelace"; got != want {
		t.Errorf("Author = %q, want %q", got, want)
	}
}
