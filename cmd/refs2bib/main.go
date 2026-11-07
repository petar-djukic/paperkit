// Command refs2bib converts a CSL-YAML reference corpus to BibTeX. The bib
// target calls the same converter in process; this command is for running it
// by hand:
//
//	go run github.com/petar-djukic/paperkit/cmd/refs2bib --yaml references.yaml --out references.bib
//
// Requires pandoc on PATH.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/petar-djukic/paperkit/bib"
)

func main() {
	yamlPath := flag.String("yaml", "references.yaml", "CSL-YAML corpus to read")
	outPath := flag.String("out", "references.bib", "BibTeX file to write")
	flag.Parse()

	count, err := bib.Convert(*yamlPath, *outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "refs2bib:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d entries) from %s\n", *outPath, count, *yamlPath)
}
