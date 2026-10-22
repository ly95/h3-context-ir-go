package compiler

import "github.com/ly95/h3-context-ir-go"

type perceptionAccounting struct {
	ObservationCards  []ObservationCardDecision
	Evidence          []contextir.Evidence
	Calls             []contextir.CallUsage
	UnmeteredAssetIDs []string
}

func completeAccounting(result *Result) {
	if len(result.UnmeteredAssetIDs) > 0 {
		result.UsageComplete = false
	}
	for _, call := range result.Calls {
		if !call.Valid {
			result.UsageComplete = false
		}
	}
}
