package compiler

import "github.com/ly95/contextir"

// Base H3 formats insert subject descriptions inline. Natural sentences must be
// composed as literal text there, rather than splicing an arbitrary definition
// sentence into a grammatical noun-phrase position. Ref2VA has explicit labels.
func verifyLiteralBaseProse(document contextir.Document) error {
	if document.Mode == contextir.ModeReference {
		return nil
	}
	check := func(text contextir.RichText, path string) error {
		for _, span := range text {
			if span.SubjectID != "" || span.ReferenceID != "" {
				return failure(contextir.CodeModelOutput, "compose", path, "Base-mode narrative must use literal text spans with natural complete subject mentions; structural subject/reference substitutions are reserved for Ref2VA.", nil)
			}
		}
		return nil
	}
	if err := check(document.Summary, "summary"); err != nil {
		return err
	}
	for _, shot := range document.Shots {
		if err := check(shot.Description, "shots.description"); err != nil {
			return err
		}
		for _, event := range shot.Events {
			if err := check(event.Description, "shots.events.description"); err != nil {
				return err
			}
		}
	}
	return nil
}
