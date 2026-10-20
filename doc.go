// Package contextir defines a versioned, provider-independent representation of
// multimodal creative intent. Documents are data: they can be edited, persisted,
// validated, and compiled without calling a model or accessing media.
//
// The planner package constructs documents using injected model and perception
// ports. The h3 package lowers documents into MiniMax H3 prompts and media
// bindings. Neither package owns generation, persistence, billing, or access
// control. ContextIR is independent of MiniMax's hosted Context-IR implementation.
package contextir
