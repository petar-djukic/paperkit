// Package wiki adds the wiki target, which builds a single HTML page from the
// paper's markdown. Import it beside papermage:
//
//	import (
//		//mage:import
//		"github.com/petar-djukic/paperkit/papermage"
//		//mage:import
//		"github.com/petar-djukic/paperkit/papermage/wiki"
//	)
//
// paper.yaml's wiki block names the template and the markdown sections.
package wiki

import "github.com/petar-djukic/paperkit"

// Wiki builds the HTML wiki from the paper's markdown.
func Wiki() error {
	configuration, err := paperkit.LoadConfig(".")
	if err != nil {
		return err
	}
	return paperkit.Wiki(".", configuration)
}
