package planner

import (
	"slices"

	"github.com/ly95/h3-context-ir-go"
)

func cloneRequest(request contextir.Request) contextir.Request {
	request.LoRAs = slices.Clone(request.LoRAs)
	for i := range request.LoRAs {
		request.LoRAs[i].Triggers = slices.Clone(request.LoRAs[i].Triggers)
	}
	request.Assets = slices.Clone(request.Assets)
	request.Constraints = slices.Clone(request.Constraints)
	request.PinnedShots = slices.Clone(request.PinnedShots)
	request.OnscreenText = slices.Clone(request.OnscreenText)
	if request.AdditionScope != nil {
		value := *request.AdditionScope
		request.AdditionScope = &value
	}
	request.Bindings = slices.Clone(request.Bindings)
	for i := range request.Bindings {
		request.Bindings[i].Source = cloneSource(request.Bindings[i].Source)
		if anchor := request.Bindings[i].Anchor; anchor != nil {
			value := *anchor
			request.Bindings[i].Anchor = &value
		}
	}
	request.Dialogue = slices.Clone(request.Dialogue)
	for i := range request.Dialogue {
		request.Dialogue[i].SpeakerIDs = slices.Clone(request.Dialogue[i].SpeakerIDs)
	}
	return request
}

func cloneEvidence(evidence []contextir.Evidence) []contextir.Evidence {
	result := slices.Clone(evidence)
	for i := range result {
		if result[i].Card != nil {
			b := *result[i].Card
			b.Subjects = slices.Clone(b.Subjects)
			for j := range b.Subjects {
				b.Subjects[j].Attributes = slices.Clone(b.Subjects[j].Attributes)
				b.Subjects[j].Pose = slices.Clone(b.Subjects[j].Pose)
			}
			b.Palette = slices.Clone(b.Palette)
			b.VisibleText = slices.Clone(b.VisibleText)
			if b.Coverage != nil {
				coverage := *b.Coverage
				coverage.Tracks = slices.Clone(coverage.Tracks)
				coverage.AbsentTracks = slices.Clone(coverage.AbsentTracks)
				coverage.SampleTimes = slices.Clone(coverage.SampleTimes)
				coverage.Intervals = slices.Clone(coverage.Intervals)
				b.Coverage = &coverage
			}
			result[i].Card = &b
		}
		result[i].Limitations = slices.Clone(result[i].Limitations)
		result[i].Facts = slices.Clone(result[i].Facts)
		for j := range result[i].Facts {
			result[i].Facts[j].Source = cloneSource(result[i].Facts[j].Source)
		}
		if coverage := result[i].Coverage; coverage != nil {
			value := *coverage
			value.Tracks = slices.Clone(coverage.Tracks)
			value.AbsentTracks = slices.Clone(coverage.AbsentTracks)
			value.SampleTimes = slices.Clone(coverage.SampleTimes)
			value.Intervals = slices.Clone(coverage.Intervals)
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
