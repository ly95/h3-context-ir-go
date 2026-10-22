package lora

import (
	"github.com/ly95/h3-context-ir-go/h3check"
	"io"
)

type Trigger struct {
	Text      string `json:"text"`
	Required  bool   `json:"required"`
	Placement string `json:"placement"`
	Count     int    `json:"count"`
}
type Strength struct {
	Default float64 `json:"default"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}
type Record struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Target        string         `json:"target"`
	H3Variant     []string       `json:"h3_variant"`
	File          string         `json:"file"`
	SHA256        string         `json:"sha256"`
	Version       int            `json:"version"`
	Triggers      []Trigger      `json:"triggers"`
	Strength      Strength       `json:"strength"`
	Constrains    map[string]any `json:"constrains"`
	ConflictsWith []string       `json:"conflicts_with"`
	Body          string         `json:"body"`
	SourcePath    string         `json:"source_path"`
}
type Request struct {
	ID       string   `json:"id"`
	Strength *float64 `json:"strength,omitempty"`
}

type Brief struct {
	Aspect  string  `json:"aspect"`
	Seconds float64 `json:"seconds"`
	Frames  int     `json:"frames,omitempty"`
	Canvas  *[2]int `json:"canvas,omitempty"`
	Steps   *int    `json:"steps,omitempty"`
}
type Choice struct {
	ID                string    `json:"id"`
	Version           int       `json:"version"`
	FileSHA256        string    `json:"file_sha256"`
	StrengthRequested float64   `json:"strength_requested"`
	StrengthApplied   float64   `json:"strength_applied"`
	Triggers          []Trigger `json:"triggers"`
	RegistryRevision  string    `json:"registry_revision"`
	ActualFileSHA256  string    `json:"actual_file_sha256,omitempty"`
	WeightsURI        string    `json:"weights_uri,omitempty"`
	Checkpoint        string    `json:"checkpoint,omitempty"`
}

type Registry struct {
	Records  map[string]Record                   `json:"records"`
	Findings []h3check.Finding                   `json:"findings"`
	Revision string                              `json:"revision"`
	OpenFile func(string) (io.ReadCloser, error) `json:"-"`
}
type PublicRecord struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Target        string         `json:"target"`
	Variants      []string       `json:"variants"`
	Strength      Strength       `json:"strength"`
	Constrains    map[string]any `json:"constrains"`
	ConflictsWith []string       `json:"conflicts_with"`
	WhatFor       string         `json:"what_for"`
	WhenNot       string         `json:"when_not"`
}
