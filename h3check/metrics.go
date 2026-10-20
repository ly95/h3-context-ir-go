package h3check

// ProseSignature exposes the upstream advisory output-prose signature for
// offline metric parity. It is not an intent detector or an authorization rule.
// Its distinctness score must remain report-only.
func ProseSignature(text string) [3]string { return signature(text) }
