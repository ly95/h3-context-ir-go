package planner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/ly95/contextir"
)

func (p *Planner) collectEvidence(ctx context.Context, assets []contextir.Asset, supplied []contextir.Evidence) ([]contextir.Evidence, error) {
	assetByID := make(map[string]contextir.Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}
	byID := make(map[string]contextir.Evidence, len(supplied))
	facts := make(map[string]contextir.Fact)
	for i, observation := range supplied {
		path := fmt.Sprintf("evidence[%d]", i)
		asset, exists := assetByID[observation.AssetID]
		if !exists {
			return nil, issueError(contextir.CodePerception, "perception", path+".asset_id", "Evidence must belong to an input asset.", nil)
		}
		if _, exists := byID[observation.AssetID]; exists {
			return nil, issueError(contextir.CodePerception, "perception", path+".asset_id", "Each asset must have exactly one evidence record.", nil)
		}
		if err := p.validateEvidence(asset, observation, facts, path); err != nil {
			return nil, err
		}
		byID[observation.AssetID] = observation
	}
	if isNil(p.perceiver) {
		for i, asset := range assets {
			if _, exists := byID[asset.ID]; !exists {
				return nil, issueError(contextir.CodePerception, "perception", fmt.Sprintf("assets[%d]", i), "This asset needs supplied evidence or a perception adapter.", nil)
			}
		}
	}
	var collected []contextir.Evidence
	for i, asset := range assets {
		if err := canceled(ctx, "perception"); err != nil {
			return collected, err
		}
		observation, exists := byID[asset.ID]
		if !exists {
			var err error
			observation, err = p.perceiver.Perceive(ctx, asset)
			if contextErr := canceled(ctx, "perception"); contextErr != nil {
				return collected, contextErr
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return collected, issueError(contextir.CodeCanceled, "perception", fmt.Sprintf("assets[%d]", i), "Perception was canceled.", err)
			}
			if err != nil {
				code := contextir.CodePerception
				if contextir.HasCode(err, contextir.CodeUnsupported) {
					code = contextir.CodeUnsupported
				}
				return collected, issueError(code, "perception", fmt.Sprintf("assets[%d]", i), "Perception could not inspect this asset.", err)
			}
			if err := p.validateEvidence(asset, observation, facts, fmt.Sprintf("evidence[%d]", i)); err != nil {
				return collected, err
			}
			observation = cloneEvidence([]contextir.Evidence{observation})[0]
		}
		collected = append(collected, observation)
	}
	return collected, nil
}

func (p *Planner) validateEvidence(asset contextir.Asset, observation contextir.Evidence, knownFacts map[string]contextir.Fact, path string) error {
	if observation.AssetID != asset.ID {
		return issueError(contextir.CodePerception, "perception", path+".asset_id", "Evidence must identify the asset that was inspected.", nil)
	}
	if strings.TrimSpace(observation.AnalyzerVersion) == "" {
		return issueError(contextir.CodePerception, "perception", path+".analyzer_version", "Evidence must identify its analyzer version.", nil)
	}
	if len(observation.Facts) == 0 {
		return issueError(contextir.CodePerception, "perception", path+".facts", "Evidence must contain explicit observations, including silence or empty content when applicable.", nil)
	}
	if err := contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{observation}); err != nil {
		var typed *contextir.Error
		if errors.As(err, &typed) && typed != nil {
			issues := make([]contextir.Issue, len(typed.Issues))
			for i, issue := range typed.Issues {
				issue.Path = strings.Replace(issue.Path, "evidence[0]", path, 1)
				issues[i] = issue
			}
			return &contextir.Error{Code: contextir.CodePerception, Stage: "perception", Issues: issues, Cause: err}
		}
		return issueError(contextir.CodePerception, "perception", path, "Evidence must satisfy the observation schema.", err)
	}
	if err := p.validateCoveragePolicy(observation, path); err != nil {
		return err
	}
	for i, fact := range observation.Facts {
		factPath := fmt.Sprintf("%s.facts[%d]", path, i)
		if strings.TrimSpace(fact.ID) == "" || strings.TrimSpace(fact.Text) == "" {
			return issueError(contextir.CodePerception, "perception", factPath, "Each observed fact needs a nonempty ID and text.", nil)
		}
		if _, exists := knownFacts[fact.ID]; exists {
			return issueError(contextir.CodePerception, "perception", factPath+".id", "Fact IDs must be unique across all evidence.", nil)
		}
		knownFacts[fact.ID] = fact
	}
	return nil
}

// Root validation owns coverage shape, required tracks, and fact-track checks.
// The planner additionally requires callers to explicitly admit limited evidence.
func (p *Planner) validateCoveragePolicy(observation contextir.Evidence, path string) error {
	coverage := observation.Coverage
	if coverage == nil {
		if p.options.AllowUnspecifiedCoverage {
			return nil
		}
		return issueError(contextir.CodePerception, "perception", path+".coverage", "Declare perception coverage or explicitly enable AllowUnspecifiedCoverage for known legacy limitations.", nil)
	}
	if coverage.Status == contextir.CoveragePartial && !p.options.AllowPartialPerception {
		return issueError(contextir.CodePerception, "perception", path+".coverage.status", "Partial perception requires explicit AllowPartialPerception; unsupported modalities must not be treated as understood.", nil)
	}
	return nil
}

func perceptionError(cause error) *contextir.Error {
	var typed *contextir.Error
	if errors.As(cause, &typed) && typed != nil && len(typed.Issues) > 0 {
		return &contextir.Error{Code: contextir.CodePerception, Stage: "perception", Issues: append([]contextir.Issue(nil), typed.Issues...), Cause: cause}
	}
	return issueError(contextir.CodePerception, "perception", "evidence", "Evidence must satisfy the observation schema.", cause)
}

func validateLocked(request contextir.Request, document contextir.Document, evidence []contextir.Evidence) error {
	if document.Mode != request.Mode {
		return issueError(contextir.CodeModelOutput, "plan", "mode", "Copy the explicit request mode unchanged.", nil)
	}
	if document.Target != request.Target {
		return issueError(contextir.CodeModelOutput, "plan", "target", "Copy every explicit target field unchanged.", nil)
	}
	if len(document.Assets) != len(request.Assets) {
		return issueError(contextir.CodeModelOutput, "plan", "assets", "Copy every original asset without adding or removing assets.", nil)
	}
	for i, asset := range request.Assets {
		if document.Assets[i] != asset {
			return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("assets[%d]", i), "Copy the original asset in its original position with all fields unchanged.", nil)
		}
	}
	if len(document.Constraints) != len(request.Constraints) {
		return issueError(contextir.CodeModelOutput, "plan", "constraints", "Copy the complete explicit constraint array without adding or removing constraints.", nil)
	}
	for i, constraint := range request.Constraints {
		if document.Constraints[i] != constraint {
			return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("constraints[%d]", i), "Copy each explicit constraint in its original position with all fields unchanged.", nil)
		}
	}
	lines := make(map[string]contextir.DialogueLine, len(document.Dialogue))
	for _, line := range document.Dialogue {
		lines[line.ID] = line
	}
	for i, line := range request.Dialogue {
		if actual, exists := lines[line.ID]; !exists || !reflect.DeepEqual(actual, line) {
			return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("dialogue[%d]", i), "Preserve the complete supplied dialogue line, including exact text, speakers, language, and delivery.", nil)
		}
	}
	if !request.AllowGeneratedDialogue {
		requested := make(map[string]bool, len(request.Dialogue))
		for _, line := range request.Dialogue {
			requested[line.ID] = true
		}
		for i, line := range document.Dialogue {
			if !requested[line.ID] {
				return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("dialogue[%d]", i), "Additional dialogue requires structured Request.Dialogue or explicit AllowGeneratedDialogue. Supply free-form dialogue as structured input; it is never silently removed.", nil)
			}
		}
	}
	if err := validateBindings(request.Bindings, document); err != nil {
		return err
	}
	if len(document.Evidence) != 0 && !reflect.DeepEqual(document.Evidence, evidence) {
		return issueError(contextir.CodeModelOutput, "plan", "evidence", "Do not alter or invent evidence; omit it so the host can attach the original records.", nil)
	}
	return validateProvenance(request, document, evidence)
}

type bindingKey struct {
	source sourceKey
	role   contextir.Role
}

func binding(source contextir.SourceRef, role contextir.Role) bindingKey {
	return bindingKey{source: key(source), role: role}
}

func validateBindings(bindings []contextir.AssetBinding, document contextir.Document) error {
	if len(bindings) == 0 {
		return nil // Free-form role interpretation remains the model's responsibility.
	}
	allowed := make(map[bindingKey][]int, len(bindings))
	used := make([]bool, len(bindings))
	for i, item := range bindings {
		value := binding(item.Source, item.Role)
		allowed[value] = append(allowed[value], i)
	}
	check := func(source contextir.SourceRef, role contextir.Role, anchor *contextir.Time, path string) error {
		permitted := false
		for _, i := range allowed[binding(source, role)] {
			// Only an explicitly supplied anchor locks target time. Other roles
			// may share a reference carrying an independent keyframe role/anchor.
			fixed := bindings[i].Anchor
			if fixed == nil || anchor != nil && *fixed == *anchor {
				used[i], permitted = true, true
			}
		}
		if !permitted {
			return issueError(contextir.CodeModelOutput, "plan", path, "Use only the exact sources, roles, source intervals, and anchors authorized by Request.Bindings.", nil)
		}
		return nil
	}
	for i, subject := range document.Subjects {
		for j, attribution := range subject.Sources {
			if err := check(attribution.Source, attribution.Role, nil, fmt.Sprintf("subjects[%d].sources[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	for i, reference := range document.References {
		for j, role := range reference.Roles {
			if err := check(reference.Source, role, reference.Anchor, fmt.Sprintf("references[%d].roles[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	for i := range bindings {
		if !used[i] {
			return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("bindings[%d]", i), "Preserve every explicit Request.Binding in the document; do not silently omit a requested source responsibility.", nil)
		}
	}
	return nil
}

type sourceKey struct {
	assetID  string
	track    contextir.Track
	interval bool
	start    contextir.Time
	end      contextir.Time
}

func key(source contextir.SourceRef) sourceKey {
	k := sourceKey{assetID: source.AssetID, track: source.Track}
	if source.Interval != nil {
		k.interval = true
		k.start = source.Interval.Start
		k.end = source.Interval.End
	}
	return k
}

func validateProvenance(request contextir.Request, document contextir.Document, evidence []contextir.Evidence) error {
	facts := make(map[string]contextir.Fact)
	sources := make(map[sourceKey]bool)
	for _, observation := range evidence {
		for _, fact := range observation.Facts {
			facts[fact.ID] = fact
			sources[key(fact.Source)] = true
		}
	}
	for i, claim := range document.Claims {
		path := fmt.Sprintf("claims[%d]", i)
		if claim.Origin == contextir.OriginCreative && !request.AllowCreativeAdditions {
			return issueError(contextir.CodeModelOutput, "plan", path+".origin", "Creative additions were not authorized by the request.", nil)
		}
		if claim.Origin == contextir.OriginObserved {
			if len(claim.Sources) == 0 {
				return issueError(contextir.CodeModelOutput, "plan", path+".sources", "Observed claims must cite supplied fact sources.", nil)
			}
			for j, source := range claim.Sources {
				if !sources[key(source)] {
					return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("%s.sources[%d]", path, j), "Observed claim sources must exactly match supplied fact sources.", nil)
				}
			}
		}
	}
	for i, subject := range document.Subjects {
		for j, attribution := range subject.Sources {
			for k, factID := range attribution.FactIDs {
				fact, exists := facts[factID]
				if !exists || key(fact.Source) != key(attribution.Source) {
					return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("subjects[%d].sources[%d].fact_ids[%d]", i, j, k), "Attribution facts must exist and match the attribution's exact source.", nil)
				}
			}
		}
	}
	return nil
}
