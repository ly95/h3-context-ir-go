package planner

import "github.com/ly95/contextir"

func copySlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append(make([]T, 0, len(values)), values...)
}

func cloneRequest(request contextir.Request) contextir.Request {
	request.Assets = copySlice(request.Assets)
	request.Constraints = copySlice(request.Constraints)
	request.Dialogue = copySlice(request.Dialogue)
	for i := range request.Dialogue {
		request.Dialogue[i].SpeakerIDs = copySlice(request.Dialogue[i].SpeakerIDs)
	}
	return request
}

func cloneEvidence(evidence []contextir.Evidence) []contextir.Evidence {
	result := copySlice(evidence)
	for i := range result {
		result[i].Limitations = copySlice(result[i].Limitations)
		result[i].Facts = copySlice(result[i].Facts)
		for j := range result[i].Facts {
			if source := &result[i].Facts[j].Source; source.Interval != nil {
				interval := *source.Interval
				source.Interval = &interval
			}
		}
	}
	return result
}
