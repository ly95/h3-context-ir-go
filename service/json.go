package service

import "github.com/ly95/h3-context-ir-go/internal/strictjson"

// Duplicate JSON fields are ambiguous requests, including inside explicitly
// open metadata maps. Reject before decoding can replace an earlier value.
func uniqueJSON(data []byte) error {
	return strictjson.Check(data, 512)
}
