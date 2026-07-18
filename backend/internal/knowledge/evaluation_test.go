// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

func TestEvaluateOverlaysInputAndReturnsOnlyResult(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": `package main
evaluations: enterpriseDiscount: {
	description: "Evaluate enterprise discount eligibility"
	input: {customerId: string, seats: int & >=0}
	result: {
		eligible: input.seats >= 100
		discountPercent: 15
	}
}
`})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	result, err := runtime.Eval(context.Background(), ProjectRef{ModuleDir: dir}, EvalRequest{
		Evaluation: "enterpriseDiscount",
		Input:      []byte(`{"customerId":"acme","seats":120}`),
	})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if result.Status != "success" || result.Evaluation != "enterpriseDiscount" || string(result.Result) != `{"eligible":true,"discountPercent":15}` || result.Revision == "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestEvaluateRejectsInputOutsideCueSchema(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": `package main
evaluations: seats: {description: "x", input: {seats: int & >=0}, result: {ok: true}}
`})
	runtime := NewRuntime(New(evaluation.New("", time.Second, 1<<20)))
	_, err := runtime.Eval(context.Background(), ProjectRef{ModuleDir: dir}, EvalRequest{Evaluation: "seats", Input: []byte(`{"seats":-1}`)})
	if err == nil {
		t.Fatal("invalid evaluation input succeeded")
	}
}
