// Package contextir defines the versioned document model for H3 Context-IR Go,
// an unofficial, independent Context-IR implementation for MiniMax H3.
// Documents are data: they can be edited, persisted,
// validated, and compiled without calling a model or accessing media.
//
// The planner package constructs documents using injected model and perception
// ports. The h3 package lowers documents into MiniMax H3 prompts and media
// bindings. Neither package owns generation, persistence, billing, or access
// control. H3 Context-IR Go is independent of MiniMax's hosted implementation.
package contextir
