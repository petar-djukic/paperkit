// Package mdtotex converts Obsidian-native markdown manuscripts into IEEEtran
// LaTeX.
//
// Convert renders one chapter as a fragment the container document inputs.
// Markdown is the source of truth: chapter and figure file names match
// one-to-one across the two formats, a caption is written once and reaches
// the LaTeX as a plain caption command, and the emitted forms stay close to
// what an author hand-writes.
//
// The package was developed as the separate md-to-tex repository, whose
// requirement documents govern the conversion; components cite them by SRD
// id (srd002-renderer-core and so on). Those documents stay with the archived
// repository, and their worked examples are pinned in testdata/examples.yaml.
package mdtotex
