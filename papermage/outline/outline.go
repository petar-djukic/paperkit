// Package outline adds the outline target to a paper drafted from section
// requirements documents. Import it beside papermage:
//
//	import (
//		//mage:import
//		"github.com/petar-djukic/paperkit/papermage"
//		//mage:import
//		"github.com/petar-djukic/paperkit/papermage/outline"
//	)
//
// paper.yaml's outline.format chooses a tex or an md outline.
package outline

import "github.com/petar-djukic/paperkit"

// Outline regenerates the outline from docs/srd/ and compiles it.
func Outline() error {
	configuration, err := paperkit.LoadConfig(".")
	if err != nil {
		return err
	}
	return paperkit.Outline(".", configuration)
}
