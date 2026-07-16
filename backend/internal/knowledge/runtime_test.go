// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package knowledge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

func TestSafeQueryFiltersAndProjectsRecords(t *testing.T) {
	dir := testModule(t, map[string]string{
		"data.cue": `package main
customers: [ID=string]: {name: string, country: string, spend: number}
customers: {
	acme: {name: "Acme", country: "FR", spend: 120}
	globex: {name: "Globex", country: "US", spend: 40}
}
`,
	})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	result, err := runtime.Query(context.Background(), ProjectRef{ModuleDir: dir}, Query{
		Domain: "customers", Select: []string{"name", "spend"}, Limit: 1,
		Where: []Predicate{{Field: "spend", Operator: "gte", Value: float64(100)}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("count = %d, want 1", result.Count)
	}
	var records []map[string]any
	if err := json.Unmarshal(result.Result, &records); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(records) != 1 || records[0]["id"] != "acme" || records[0]["name"] != "Acme" || records[0]["country"] != nil {
		t.Fatalf("records = %+v", records)
	}
}

func TestSafeQueryRejectsUnknownFieldsAndExpressions(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": "package main\ncustomers: [string]: {name: string}\ncustomers: {acme: {name: \"Acme\"}}\n"})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	_, err := runtime.Query(context.Background(), ProjectRef{ModuleDir: dir}, Query{Domain: "customers", Where: []Predicate{{Field: "unknown", Operator: "eq", Value: "x"}}})
	if err == nil {
		t.Fatal("unknown field query succeeded")
	}
}
