package evaluate

import "testing"

// Source test_deterministic.py::test_no_rule_id_carries_two_meanings. Reuse the
// actual Go AST scanner: it scans production literals across native namespaces,
// checks non-vacuous discovery and rejects prefix/number meaning collisions.
func TestRemainingSurfaceRuleIDSemanticUniqueness(t *testing.T) {
	TestProductionRuleNumbersHaveSingleMeanings(t)
}
