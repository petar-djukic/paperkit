// Command genoutline prints a paper's outline, generated from its section
// requirements documents, to stdout. Run it from the paper directory:
//
//	go run github.com/petar-djukic/paperkit/cmd/genoutline [-docs docs] [-format tex|md]
//
// The mage outline target (papermage/outline) calls the same renderer and also
// compiles the result.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/petar-djukic/paperkit/srd"
)

func main() {
	docs := flag.String("docs", "docs", "paper docs directory holding VISION.md and srd/")
	format := flag.String("format", "tex", "output format: tex (a standalone IEEEtran document) or md (a markdown chapter for the mdtotex converter)")
	flag.Parse()

	outline, err := srd.Render(*docs, srd.Format(*format))
	if err != nil {
		fmt.Fprintln(os.Stderr, "genoutline:", err)
		os.Exit(1)
	}
	os.Stdout.Write(outline)
}
