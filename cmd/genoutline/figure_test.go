package main

import "testing"

func TestFigureArtifactMapsD2SourcesToTheirCompiledPDF(t *testing.T) {
	for declared, want := range map[string]string{
		"d2/whole-system-overview.d2": "whole-system-overview.pdf",
		"fig/01-kinds-to-levels.pdf":  "01-kinds-to-levels.pdf",
		"images/x.png":                "x.png",
		"bare.d2":                     "bare.pdf",
	} {
		if got := figureArtifact(declared); got != want {
			t.Errorf("figureArtifact(%q) = %q, want %q", declared, got, want)
		}
	}
}

func TestOkeyOrdersAppendicesAfterTheBody(t *testing.T) {
	units := []string{"SA", "S1", "SC", "S2.3", "S9", "SB", "S2"}
	want := []string{"S1", "S2", "S2.3", "S9", "SA", "SB", "SC"}
	sortUnits(units)
	for i := range want {
		if units[i] != want[i] {
			t.Fatalf("order = %v, want %v", units, want)
		}
	}
}
