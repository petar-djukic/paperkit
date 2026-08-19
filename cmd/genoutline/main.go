// Command genoutline generates outline.tex from the SRDs (GH-94, ported from
// gen_outline.py in GH-217).
//
// Reads <docs>/VISION.yaml (abstract), <docs>/constitutions/argument.yaml
// (the title of record) and <docs>/srd/*.yaml (units in
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

// okey orders unit ids: S1 -> (1,0), S2.3 -> (2,3).
func okey(u string) (int, int) {
	p := strings.SplitN(u[1:], ".", 2)
	major, _ := strconv.Atoi(p[0])
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

func main() {
	docs := flag.String("docs", "docs", "paper docs directory holding VISION.yaml and srd/")
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
	sort.Slice(units, func(i, j int) bool {
		ai, bi := okey(units[i])
		aj, bj := okey(units[j])
		if ai != aj {
			return ai < aj
		}
		return bi < bj
	})

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
	// The title of record is argument.yaml meta.paper, which carries the
	// subtitle; VISION meta.artifact describes the artifact, not the paper, and
	// 00-front-matter.md reads argument.yaml too. One source, no drift (GH-393).
	argPath := filepath.Join(*docs, "constitutions", "argument.yaml")
	arg, err := loadYAML(argPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genoutline:", err)
		os.Exit(1)
	}
	title := getStr(m(arg["meta"]), "paper")
	if title == "" {
		fmt.Fprintf(os.Stderr, "genoutline: %s: meta.paper is empty; it is the title of record\n", argPath)
		os.Exit(1)
	}
	w(`\title{Outline: ` + esc(title) + `}`)
	w(`\author{Petar Djukic%`)
	w(`\thanks{Outline generated from the section requirements documents (SRDs) of ieee-comst; regenerate with go run ../cmd/genoutline.}}`)
	w(`\markboth{Working outline, ` + esc(startedDisplay(visMeta["started"])) + `}{Djukic: Outline}`)
	w(`\maketitle`)
	w(`\begin{abstract}`)
	w(esc(strings.Join(strings.Fields(getStr(vis, "abstract")), " ")))
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
				file := getStr(fgm, "file")
				parts := strings.Split(file, "/")
				w(`\includegraphics[width=\columnwidth]{` + parts[len(parts)-1] + `}`)
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
