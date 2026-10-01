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
