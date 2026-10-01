// Command genoutline generates outline.tex from the SRDs (GH-94, ported from
// gen_outline.py in GH-217).
//
// Reads <docs>/../00-title-page.md (the title of record and the abstract's
// source of record), <docs>/VISION.yaml (goals) and <docs>/srd/*.yaml (units in
// reading order) and emits a two-column IEEEtran outline document: per unit,
// its goal sentence and the specific subgoals (each with an id G<n>.<m>
// pointing at paper goal G<n>), and its content (the objective as a lead-in,
// then the topics it will cover), tables and figures with descriptions, how
// it contributes (links.supports), and its references via \cite.
// Regenerate whenever SRDs change:
//
//	go run ../cmd/genoutline > outline.tex
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func esc(s string) string {
	for _, p := range [][2]string{
		{"&", `\&`}, {"%", `\%`}, {"#", `\#`}, {"_", `\_`}, {"$", `\$`},
	} {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// str renders a YAML scalar the way Python's str() does for the shapes the
// SRDs contain (strings, ints, floats).
func str(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func oneLine(v interface{}) string {
	return esc(strings.Join(strings.Fields(str(v)), " "))
}

// okey orders unit ids: S1 -> (1,0), S2.3 -> (2,3). Letter units are the
// appendices (SA, SB, SC) and sort after every numbered unit, alphabetically,
// because Atoi on a letter would otherwise put them at zero, ahead of the
// body.
func okey(u string) (int, int) {
	p := strings.SplitN(u[1:], ".", 2)
	major, err := strconv.Atoi(p[0])
	if err != nil && p[0] != "" {
		major = 1<<20 + int(p[0][0])
	}
	minor := 0
	if len(p) > 1 {
		minor, _ = strconv.Atoi(p[1])
	}
	return major, minor
}

func loadYAML(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d map[string]interface{}
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func m(v interface{}) map[string]interface{} {
	if t, ok := v.(map[string]interface{}); ok {
		return t
	}
	return map[string]interface{}{}
}

func list(v interface{}) []interface{} {
	if t, ok := v.([]interface{}); ok {
		return t
	}
	return nil
}

func getStr(mm map[string]interface{}, k string) string {
	if v, ok := mm[k]; ok && v != nil {
		return str(v)
	}
	return ""
}

// startedDisplay renders meta.started as PyYAML+strftime did: a date value
// becomes "Month Year"; anything else passes through as text.
func startedDisplay(v interface{}) string {
	if t, ok := v.(time.Time); ok {
		return t.Format("January 2006")
	}
	s := str(v)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("January 2006")
	}
	return s
}

// loadFrontMatter reads the pandoc YAML metadata block at the top of
// 00-title-page.md, the title of record (GH-427; it previously lived in
// constitutions/argument.yaml) and the abstract's source of record (GH-418;
// it previously lived in VISION.yaml).
func loadFrontMatter(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil, fmt.Errorf("%s: no YAML metadata block", path)
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("%s: unterminated YAML metadata block", path)
	}
	var d map[string]interface{}
	if err := yaml.Unmarshal([]byte(rest[:end]), &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// emitMarkdown renders the outline as a markdown chapter: plain text a
// human reads in any editor (or Obsidian), and a valid md-to-tex source the
// paper's magefile compiles through the same pipeline as the chapters. The
// title and abstract stay in 00-title-page.md — the outline is a chapter,
// so the paper's own front matter renders the title page. Citations are
// pandoc-style [@id] and resolve against the paper's bibliography; planned
// tables and figures render as italic notes, and a table with drafted rows
// renders as a pipe table with the caption line md-to-tex requires.
func emitMarkdown(vis map[string]interface{}, units []string, srds map[string]map[string]interface{}) {
	var out []string
	w := func(lines ...string) { out = append(out, lines...) }

	w("<!-- Generated from docs/srd/*.yaml by cmd/genoutline -format md;")
	w("     regenerate with `mage outline`. Edit the SRDs, not this file. -->")

	for _, u := range units {
		d := srds[u]
		meta := m(d["meta"])
		title := oneLine(meta["title"])
		depth := strings.Count(u, ".")
		heading := "#"
		kind := "section"
		if depth > 0 {
			heading = "##"
			kind = "subsection"
		}
		w("", heading+" "+title)

		// In the introduction, state the paper goals first.
		if u == "S1" {
			goals := list(vis["goals"])
			w("", fmt.Sprintf("The paper has %d goals.", len(goals)), "")
			for _, gg := range goals {
				gm := m(gg)
				w("- **" + getStr(gm, "id") + "** " + oneLine(gm["goal"]))
			}
		}

		if sg := oneLine(d["section_goal"]); d["section_goal"] != nil && sg != "" {
			w("", "The goal of this "+kind+" is to "+sg+".")
		}
		if goals := list(d["goals"]); len(goals) > 0 {
			w("")
			for _, g := range goals {
				gm := m(g)
				pre := ""
				if id := getStr(gm, "id"); id != "" {
					pre = "**" + id + "** "
				}
				w("- " + pre + oneLine(gm["goal"]))
			}
		}
		if d["objective"] != nil {
			if obj := oneLine(d["objective"]); obj != "" {
				lead := strings.ToLower(obj[:1]) + obj[1:]
				w("", "The content of this "+kind+" "+lead)
			}
		}
		var arts []string
		if says := list(d["content"]); len(says) > 0 {
			w("")
			for _, t := range says {
				tm := m(t)
				if s, ok := tm["say"].(string); ok && s != "" && !strings.Contains(s, "GH-") {
					w("- " + oneLine(s))
				}
				if a, ok := tm["artifact"]; ok && a != nil && str(a) != "" {
					arts = append(arts, oneLine(a))
				}
			}
		}

		// Closing paragraph: artifacts, contribution edges, references.
		var p2 []string
		if len(arts) > 0 {
			p2 = append(p2, "Planned artifacts: "+strings.Join(arts, "; ")+".")
		}
		links := m(d["links"])
		disp := func(us []interface{}) string {
			var parts []string
			for _, x := range us {
				parts = append(parts, str(x))
			}
			return strings.Join(parts, ", ")
		}
		var edges []string
		if sup := list(links["supports"]); len(sup) > 0 {
			edges = append(edges, "feeds "+disp(sup))
		}
		if req := list(links["requires"]); len(req) > 0 {
			edges = append(edges, "builds on "+disp(req))
		}
		if len(edges) > 0 {
			p2 = append(p2, "This unit "+strings.Join(edges, "; ")+".")
		}
		var cites []string
		for _, c := range list(d["citations"]) {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if id := getStr(cm, "id"); id != "" {
				cites = append(cites, "[@"+id+"]")
			}
		}
		if len(cites) > 0 {
			p2 = append(p2, "References: "+strings.Join(cites, ", ")+".")
		}
		if len(p2) > 0 {
			w("", strings.Join(p2, " "))
		}

		for index, tb := range list(d["tables"]) {
			tbm := m(tb)
			cols := list(tbm["columns"])
			rows := list(tbm["rows"])
			if len(cols) > 0 && len(rows) > 0 {
				var hdr, sep []string
				for _, c := range cols {
					hdr = append(hdr, str(c))
					sep = append(sep, "---")
				}
				w("", "| "+strings.Join(hdr, " | ")+" |")
				w("|" + strings.Join(sep, "|") + "|")
				for _, row := range rows {
					var cells []string
					for _, x := range list(row) {
						cells = append(cells, str(x))
					}
					w("| " + strings.Join(cells, " | ") + " |")
				}
				w("", fmt.Sprintf("Table: %s {#tab:outline-%s-%d}", oneLine(tbm["shows"]), strings.ReplaceAll(u[1:], ".", "-"), index+1))
			} else {
				note := "*Table planned: " + oneLine(tbm["shows"])
				if len(cols) > 0 {
					var names []string
					for _, c := range cols {
						names = append(names, str(c))
					}
					note += " (columns: " + strings.Join(names, "; ") + ")"
				}
				w("", note+".*")
			}
		}
		for _, fg := range list(d["figures"]) {
			fgm := m(fg)
			note := "*Figure planned: " + oneLine(fgm["shows"])
			if getStr(fgm, "status") == "ready" {
				note = "*Figure ready (" + getStr(fgm, "file") + "): " + oneLine(fgm["shows"])
			}
			w("", note+".*")
		}
	}

	os.Stdout.WriteString(strings.Join(out, "\n") + "\n")
}

func main() {
	docs := flag.String("docs", "docs", "paper docs directory holding VISION.yaml and srd/")
	format := flag.String("format", "tex", "output format: tex (a standalone IEEEtran document) or md (a markdown chapter for the md-to-tex pipeline)")
	flag.Parse()

	vis, err := loadYAML(filepath.Join(*docs, "VISION.yaml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "genoutline:", err)
		os.Exit(1)
	}
	files, _ := filepath.Glob(filepath.Join(*docs, "srd", "srd-*.yaml"))
	sort.Strings(files)
	srds := map[string]map[string]interface{}{}
	for _, f := range files {
		d, err := loadYAML(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "genoutline:", err)
			os.Exit(1)
		}
		meta := m(d["meta"])
		unit := getStr(meta, "unit")
		if unit == "" {
			unit = getStr(meta, "subsection")
		}
		if unit != "" {
			srds[unit] = d
		}
	}
	units := make([]string, 0, len(srds))
	for u := range srds {
		units = append(units, u)
	}
	sortUnits(units)

	if *format == "md" {
		emitMarkdown(vis, units, srds)
		return
	}

	var out []string
	w := func(lines ...string) { out = append(out, lines...) }

	w(`\documentclass[journal]{IEEEtran}`,
		`\usepackage{cite}`,
		`\usepackage{xurl}`,
		`\usepackage{tabularx}`,
		`\usepackage{booktabs}`,
		`\usepackage{graphicx}`,
		`\usepackage{float}`,
		`\usepackage{array}`,
		// The docs use U+202F (narrow no-break space) before numerals and after
		// abbreviations -- "TM Forum", "Table 2.6", "Levels 3 and 4". Latin
		// Modern has no glyph for it, so xelatex drops it silently and the PDF
		// reads "Levels3". Map it to \, (an unbreakable thin space), which is
		// what the character means. templates/ieee-preamble.tex carries the
		// same mapping for the `mage build` path (GH-376).
		`\usepackage{newunicodechar}`,
		"\\newunicodechar{\u202f}{\\,}",
		`\renewcommand{\tabularxcolumn}[1]{>{\raggedright\arraybackslash}p{#1}}`,
		`\graphicspath{{../fig/}{fig/}}`,
		`\hyphenation{auto-gen-ic}`,
		`\begin{document}`)
	visMeta := m(vis["meta"])
	// The title of record is 00-title-page.md, which carries the subtitle;
	// VISION meta.artifact describes the artifact, not the paper. One source,
	// no drift (GH-393, retargeted from argument.yaml by GH-427).
	fmPath := filepath.Join(*docs, "..", "00-title-page.md")
	front, err := loadFrontMatter(fmPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genoutline:", err)
		os.Exit(1)
	}
	title := getStr(front, "title")
	if title == "" {
		fmt.Fprintf(os.Stderr, "genoutline: %s: title is empty; it is the title of record\n", fmPath)
		os.Exit(1)
	}
	w(`\title{Outline: ` + esc(title) + `}`)
	w(`\author{Petar Djukic%`)
	paperName := "the paper"
	if absDocs, err := filepath.Abs(*docs); err == nil {
		paperName = filepath.Base(filepath.Dir(absDocs))
	}
	w(`\thanks{Outline generated from the section requirements documents (SRDs) of ` + esc(paperName) + `; regenerate with go run ../cmd/genoutline.}}`)
	w(`\markboth{Working outline, ` + esc(startedDisplay(visMeta["started"])) + `}{Djukic: Outline}`)
	w(`\maketitle`)
	abstract := getStr(front, "abstract")
	if abstract == "" {
		fmt.Fprintf(os.Stderr, "genoutline: %s: abstract is empty; it is the source of record\n", fmPath)
		os.Exit(1)
	}
	w(`\begin{abstract}`)
	w(esc(strings.Join(strings.Fields(abstract), " ")))
	w(`\end{abstract}`)

	for _, u := range units {
		d := srds[u]
		meta := m(d["meta"])
		title := oneLine(meta["title"])
		depth := strings.Count(u, ".")
		num := u[1:]
		if depth == 0 {
			w("", `\section{`+title+`}`)
		} else {
			w("", `\subsection{`+title+`}`)
		}
		w(`\label{sec:` + num + `}`)

		org := list(d["content"])
		var says, arts []string
		for _, t := range org {
			tm := m(t)
			if s, ok := tm["say"].(string); ok && s != "" && !strings.Contains(s, "GH-") {
				says = append(says, oneLine(s))
			}
			if a, ok := tm["artifact"]; ok && a != nil && str(a) != "" {
				arts = append(arts, oneLine(a))
			}
		}
		links := m(d["links"])
		var cites []string
		for _, c := range list(d["citations"]) {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			cites = append(cites, getStr(cm, "id"))
		}

		// In the introduction, state the paper goals G1-G5 first.
		if u == "S1" {
			w("The article has five goals.")
			w(`\begin{itemize}`)
			for _, gg := range list(vis["goals"]) {
				gm := m(gg)
				w(`  \item \textbf{` + esc(getStr(gm, "id")) + `} ` + oneLine(gm["goal"]))
			}
			w(`\end{itemize}`, "")
		}
		kind := "subsection"
		if depth == 0 {
			kind = "section"
		}
		// Goals: a lead sentence, then the specific subgoals as a list.
		if sg := oneLine(d["section_goal"]); d["section_goal"] != nil && sg != "" {
			w("The goal of this " + kind + " is to " + sg + ".")
		}
		goals := list(d["goals"])
		if len(goals) > 0 {
			w(`\begin{itemize}`)
			for _, g := range goals {
				gm := m(g)
				pre := ""
				if id := getStr(gm, "id"); id != "" {
					pre = `\textbf{` + esc(id) + `} `
				}
				w(`  \item ` + pre + oneLine(gm["goal"]))
			}
			w(`\end{itemize}`)
		}
		w("")
		// Content: a lead sentence (singular 'content' agrees with the objective verbs), then the topics.
		if d["objective"] != nil {
			if obj := oneLine(d["objective"]); obj != "" {
				lead := strings.ToLower(obj[:1]) + obj[1:]
				w("The content of this " + kind + " " + lead)
			}
		}
		if len(says) > 0 {
			w(`\begin{itemize}`)
			for _, sy := range says {
				w(`  \item ` + sy)
			}
			w(`\end{itemize}`)
		}
		w("")

		// Closing paragraph: artifacts, contribution edges, references.
		var p2 []string
		if len(arts) > 0 {
			p2 = append(p2, "Planned artifacts: "+strings.Join(arts, "; ")+".")
		}
		disp := func(us []interface{}) string {
			var parts []string
			for _, x := range us {
				parts = append(parts, `\ref{sec:`+str(x)[1:]+`}`)
			}
			return strings.Join(parts, ", ")
		}
		var edges []string
		if lvlV, has := meta["level"]; has {
			lvl := str(lvlV)
			verb := "demonstrates"
			if strings.HasSuffix(lvl, "-facing") {
				verb = "works toward full autonomy with"
			}
			edges = append(edges, verb+" the "+oneLine(meta["agent_kind"])+" ("+lvl+")")
		}
		if sup := list(links["supports"]); len(sup) > 0 {
			edges = append(edges, "feeds Sections "+disp(sup))
		}
		if req := list(links["requires"]); len(req) > 0 {
			edges = append(edges, "builds on Sections "+disp(req))
		}
		if len(edges) > 0 {
			p2 = append(p2, "This unit "+strings.Join(edges, "; ")+".")
		}
		if len(cites) > 0 {
			p2 = append(p2, `References: \cite{`+strings.Join(cites, ",")+`}.`)
		}
		if len(p2) > 0 {
			w(strings.Join(p2, " "), "")
		}

		// Tables: an actual table float in the section. Wide tables span both
		// columns (table*); tabularx wraps prose cells; booktabs rules.
		for _, tb := range list(d["tables"]) {
			tbm := m(tb)
			wide := false
			if wv, has := tbm["wide"]; has {
				if b, ok := wv.(bool); ok {
					wide = b
				} else if wv != nil {
					wide = true
				}
			}
			env, place, width := "table", "[H]", `\columnwidth`
			if wide {
				env, place, width = "table*", "[!t]", `\textwidth`
			}
			w(`\begin{` + env + `}` + place)
			w(`\centering`)
			w(`\caption{` + esc(getStr(tbm, "shows")) + `}`)
			cols := list(tbm["columns"])
			rows := list(tbm["rows"])
			if len(cols) > 0 && len(rows) > 0 {
				spec := "@{}" + strings.Repeat("X", len(cols)) + "@{}"
				w(`\begin{tabularx}{` + width + `}{` + spec + `}`)
				w(`\toprule`)
				var hdr []string
				for _, c := range cols {
					hdr = append(hdr, `\textbf{`+esc(str(c))+`}`)
				}
				w(strings.Join(hdr, " & ") + ` \\`)
				w(`\midrule`)
				for _, row := range rows {
					var cells []string
					for _, x := range list(row) {
						cells = append(cells, esc(str(x)))
					}
					w(strings.Join(cells, " & ") + ` \\`)
				}
				w(`\bottomrule`)
				w(`\end{tabularx}`)
			} else {
				w(`\begin{tabular}{@{}l@{}}\hline \textit{[table planned]} \\ \hline\end{tabular}`)
			}
			w(`\end{`+env+`}`, "")
		}

		// Figures: locked in the section with [H]; ready ones embedded, planned as
		// a captioned placeholder. Single column so nothing floats out of section.
		for _, fg := range list(d["figures"]) {
			fgm := m(fg)
			w(`\begin{figure}[H]`)
			w(`\centering`)
			if getStr(fgm, "status") == "ready" {
				w(`\includegraphics[width=\columnwidth]{` + figureArtifact(getStr(fgm, "file")) + `}`)
			} else {
				w(`\fbox{\parbox[c][3\baselineskip][c]{0.85\columnwidth}{\centering\textit{[figure planned]}}}`)
			}
			w(`\caption{` + esc(getStr(fgm, "shows")) + `}`)
			w(`\end{figure}`, "")
		}
	}

	w("")
	w(`\bibliographystyle{IEEEtran}`)
	w(`\bibliography{references}`)
	w(`\end{document}`)
	os.Stdout.WriteString(strings.Join(out, "\n") + "\n")
}

// figureArtifact maps an SRD figure declaration to the file graphicx should
// include. SRDs declare figures either as the compiled artifact
// (fig/<stem>.pdf) or as the d2 source (d2/<stem>.d2); the source form is the
// more durable declaration, but LaTeX can only bound the PDF the figures
// target compiles beside it.
func figureArtifact(declared string) string {
	parts := strings.Split(declared, "/")
	name := parts[len(parts)-1]
	if strings.HasSuffix(name, ".d2") {
		return strings.TrimSuffix(name, ".d2") + ".pdf"
	}
	return name
}

// sortUnits orders unit ids by okey: body sections numerically, then the
// lettered appendices.
func sortUnits(units []string) {
	sort.Slice(units, func(i, j int) bool {
		ai, bi := okey(units[i])
		aj, bj := okey(units[j])
		if ai != aj {
			return ai < aj
		}
		return bi < bj
	})
}
