// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stratorys/cueto/backend/internal/knowledge"
)

// knowledgeFixture is a small module with one registry domain and one named
// evaluation, enough to exercise every knowledge endpoint end to end.
const knowledgeFixture = `package main

customers: [ID=string]: {name: string, country: string}
customers: {
	acme:   {name: "Acme", country: "FR"}
	globex: {name: "Globex", country: "US"}
}

evaluations: discount: {
	description: "Evaluate a seat-count discount"
	input: {seats: int & >=0}
	result: {eligible: input.seats >= 10}
}
`

func TestKnowledgeCatalogListsDomainsAndEvaluations(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/catalog"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Domains []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"domains"`
		Evaluations []struct {
			Name string `json:"name"`
		} `json:"evaluations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	foundDomain, foundEval := false, false
	for _, d := range body.Domains {
		if d.Name == "customers" && d.Kind == "registry" {
			foundDomain = true
		}
	}
	for _, e := range body.Evaluations {
		if e.Name == "discount" {
			foundEval = true
		}
	}
	if !foundDomain || !foundEval {
		t.Fatalf("body = %+v, want customers domain and discount evaluation", body)
	}
}

func TestKnowledgeDescribeReturnsMembers(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/domains/customers"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Members []string `json:"Members"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if len(body.Members) != 2 {
		t.Fatalf("members = %+v, want acme and globex", body.Members)
	}
}

func TestKnowledgeDescribeUnknownDomain(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/domains/nope"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeGetReturnsRecord(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/domains/customers/acme"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var record map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if record["name"] != "Acme" {
		t.Fatalf("record = %+v", record)
	}
}

func TestKnowledgeGetUnknownKey(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/domains/customers/nope"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeQueryFiltersRecords(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	body, err := json.Marshal(knowledge.Query{
		Domain: "customers",
		Select: []string{"name"},
		Where:  []knowledge.Predicate{{Field: "country", Operator: "eq", Value: "FR"}},
	})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	rec := postJSON(router, ppid(id, "/knowledge/query"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result knowledge.QueryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if result.Count != 1 {
		t.Fatalf("result = %+v, want one match", result)
	}
}

func TestKnowledgeQueryUnknownField(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	body, err := json.Marshal(knowledge.Query{
		Domain: "customers",
		Where:  []knowledge.Predicate{{Field: "nope", Operator: "eq", Value: "x"}},
	})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	rec := postJSON(router, ppid(id, "/knowledge/query"), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeEvalSuccess(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := postJSON(router, ppid(id, "/knowledge/eval/discount"), []byte(`{"input":{"seats":20}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result knowledge.EvalResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if result.Status != "success" || string(result.Result) != `{"eligible":true}` {
		t.Fatalf("result = %+v", result)
	}
}

func TestKnowledgeEvalUnknownName(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := postJSON(router, ppid(id, "/knowledge/eval/nope"), []byte(`{"input":{}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeProvenanceListsEntries(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/provenance"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Provenance struct {
			Entries []struct {
				Name string `json:"Name"`
			} `json:"Entries"`
		} `json:"Provenance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if len(body.Provenance.Entries) == 0 {
		t.Fatalf("body = %+v, want at least one entry", body)
	}
}

func TestKnowledgeHealthValid(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"data.cue": knowledgeFixture})
	rec := getJSON(router, ppid(id, "/knowledge/health"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var health knowledge.Health
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if !health.Valid {
		t.Fatalf("health = %+v, want valid", health)
	}
}

func TestKnowledgeCatalogUnknownProject(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := getJSON(router, ppid("does-not-exist", "/knowledge/catalog"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}
