package service

import "github.com/ly95/h3-context-ir-go/internal/strictjson"

func uniqueJSON(data []byte) error {
	return strictjson.Check(data, 512)
}
