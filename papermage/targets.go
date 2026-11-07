// Package papermage is the mage target set a paper directory imports instead
// of writing its own. A paper's whole magefile can be:
//
//	//go:build mage
//
//	package main
//
//	import (
//		//mage:import
//		"github.com/petar-djukic/paperkit/papermage"
//	)
//
//	var Default = papermage.PDF
//
// Every target reads paper.yaml from the working directory. A paper with
// checks of its own registers them with BeforeAudit from its magefile's init
// function; mage rejects a local target that shares an imported target's
// name, so a paper extends Audit rather than redefining it. The outline and
// wiki targets live in the papermage/outline and papermage/wiki packages, so a
// paper lists only the targets it uses.
package papermage

import (
	"fmt"

	"github.com/petar-djukic/paperkit"
)

var auditChecks []func() error

// BeforeAudit registers a paper-specific check that Audit runs before it
// regenerates and audits the build. Call it from the magefile's init.
func BeforeAudit(check func() error) {
	auditChecks = append(auditChecks, check)
}

func config() (paperkit.Config, error) {
	return paperkit.LoadConfig(".")
}

// Tex regenerates tex/ from the markdown chapters.
func Tex() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	return paperkit.GenerateTex(".", configuration)
}

// Figures compiles every d2 source to PDF, skipping those already current.
func Figures() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	return paperkit.Figures(".", configuration)
}

// Diagrams renders every d2 source to PNG for markdown pages (stale only).
func Diagrams() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	if configuration.Diagrams == "" {
		fmt.Println("diagrams: paper.yaml sets no diagrams directory")
		return nil
	}
	return paperkit.Diagrams(".", configuration)
}

// Bib regenerates tex/references.bib from the paper's references.yaml.
func Bib() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	return paperkit.Bib(".", configuration)
}

// PDF builds the paper: figures, diagrams, bibliography, LaTeX, then latexmk.
func PDF() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	if err := paperkit.Figures(".", configuration); err != nil {
		return err
	}
	if err := paperkit.Diagrams(".", configuration); err != nil {
		return err
	}
	if err := paperkit.Bib(".", configuration); err != nil {
		return err
	}
	if err := paperkit.GenerateTex(".", configuration); err != nil {
		return err
	}
	return paperkit.PDF(".", configuration)
}

// Audit runs the paper's checks, then audits sources, references and a build.
func Audit() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	for _, check := range auditChecks {
		if err := check(); err != nil {
			return err
		}
	}
	// tex/ is build output, so a fresh checkout has none: generate it first,
	// or the audit reports every chapter missing.
	if err := paperkit.Bib(".", configuration); err != nil {
		return err
	}
	if err := paperkit.GenerateTex(".", configuration); err != nil {
		return err
	}
	return paperkit.Audit(".", configuration)
}

// Clean removes build/, tex/, and the compiled figure PDFs.
func Clean() error {
	configuration, err := config()
	if err != nil {
		return err
	}
	return paperkit.Clean(".", configuration)
}
