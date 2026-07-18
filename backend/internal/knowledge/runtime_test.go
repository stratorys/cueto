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

const describeGetFixture = `package main
customers: [ID=string]: {name: string, country: string}
customers: {
	acme: {name: "Acme", country: "FR"}
	globex: {name: "Globex", country: "US"}
}
`

func TestDescribeReturnsDomainAndMembers(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	result, err := runtime.Describe(context.Background(), ProjectRef{ModuleDir: dir}, "customers")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if result.Kind != "registry" || len(result.Members) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestDescribeUnknownDomain(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	if _, err := runtime.Describe(context.Background(), ProjectRef{ModuleDir: dir}, "nope"); err == nil {
		t.Fatal("unknown domain succeeded")
	}
}

func TestGetReturnsOneRecord(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	result, err := runtime.Get(context.Background(), ProjectRef{ModuleDir: dir}, "customers", "acme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(result, &record); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if record["name"] != "Acme" {
		t.Fatalf("record = %+v", record)
	}
}

func TestGetUnknownKey(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	if _, err := runtime.Get(context.Background(), ProjectRef{ModuleDir: dir}, "customers", "nope"); err == nil {
		t.Fatal("unknown key succeeded")
	}
}

func TestProvenanceListsDeclarationsAndFiltersByName(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	all, err := runtime.Provenance(context.Background(), ProjectRef{ModuleDir: dir}, "")
	if err != nil {
		t.Fatalf("Provenance: %v", err)
	}
	if len(all.Provenance.Entries) == 0 {
		t.Fatalf("provenance = %+v, want at least one entry", all)
	}
	filtered, err := runtime.Provenance(context.Background(), ProjectRef{ModuleDir: dir}, "customers")
	if err != nil {
		t.Fatalf("Provenance(customers): %v", err)
	}
	// The fixture declares "customers" twice (the registry pattern and the
	// concrete members), so both declaration sites are expected back.
	if len(filtered.Provenance.Entries) != 2 || filtered.Provenance.Entries[0].Name != "customers" {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestProvenanceUnknownName(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": describeGetFixture})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	if _, err := runtime.Provenance(context.Background(), ProjectRef{ModuleDir: dir}, "nope"); err == nil {
		t.Fatal("unknown name succeeded")
	}
}

func TestHealthReflectsModuleValidity(t *testing.T) {
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	valid := testModule(t, map[string]string{"data.cue": describeGetFixture})
	result, err := runtime.Health(context.Background(), ProjectRef{ModuleDir: valid})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %+v, want a clean module", result)
	}

	invalid := testModule(t, map[string]string{"data.cue": "package main\nbad: 1\nbad: \"x\"\n"})
	result, err = runtime.Health(context.Background(), ProjectRef{ModuleDir: invalid})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if result.Valid || len(result.Diagnostics) == 0 {
		t.Fatalf("result = %+v, want an invalid module", result)
	}
}
