package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

var callbackInput *bufio.Scanner
var callbackOutput *json.Encoder

type callbackReply struct {
	Value json.RawMessage `json:"value"`
	Error *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
		Code    string `json:"code"`
		Status  int    `json:"status"`
	} `json:"error"`
}

func invokePythonCallback(name string, payload any) (callbackReply, error) {
	if callbackInput == nil || callbackOutput == nil {
		return callbackReply{}, fmt.Errorf("test callback channel is unavailable")
	}
	if e := callbackOutput.Encode(map[string]any{"callback": name, "payload": payload}); e != nil {
		return callbackReply{}, e
	}
	for {
		if !callbackInput.Scan() {
			if e := callbackInput.Err(); e != nil {
				return callbackReply{}, e
			}
			return callbackReply{}, fmt.Errorf("test callback channel closed")
		}
		var envelope map[string]json.RawMessage
		if e := json.Unmarshal(callbackInput.Bytes(), &envelope); e != nil {
			return callbackReply{}, e
		}
		if _, nested := envelope["operation"]; nested {
			var req struct {
				Operation string          `json:"operation"`
				Arguments json.RawMessage `json:"arguments"`
			}
			decoder := json.NewDecoder(bytes.NewReader(callbackInput.Bytes()))
			decoder.DisallowUnknownFields()
			if e := decoder.Decode(&req); e != nil {
				return callbackReply{}, e
			}
			if e := callbackOutput.Encode(callOperation(req.Operation, req.Arguments)); e != nil {
				return callbackReply{}, e
			}
			continue
		}
		var reply callbackReply
		if e := json.Unmarshal(callbackInput.Bytes(), &reply); e != nil {
			return callbackReply{}, fmt.Errorf("invalid callback reply: %w", e)
		}
		return reply, nil
	}
}
