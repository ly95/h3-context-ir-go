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
	request.Bindings = copySlice(request.Bindings)
	for i := range request.Bindings {
		request.Bindings[i].Source = cloneSource(request.Bindings[i].Source)
		if anchor := request.Bindings[i].Anchor; anchor != nil {
			value := *anchor
			request.Bindings[i].Anchor = &value
		}
	}
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
			result[i].Facts[j].Source = cloneSource(result[i].Facts[j].Source)
		}
		if coverage := result[i].Coverage; coverage != nil {
			value := *coverage
			value.Tracks = copySlice(coverage.Tracks)
			value.AbsentTracks = copySlice(coverage.AbsentTracks)
			value.SampleTimes = copySlice(coverage.SampleTimes)
			value.Intervals = copySlice(coverage.Intervals)
			result[i].Coverage = &value
		}
	}
	return result
}

func cloneSource(source contextir.SourceRef) contextir.SourceRef {
	if source.Interval != nil {
		value := *source.Interval
		source.Interval = &value
	}
	return source
}
