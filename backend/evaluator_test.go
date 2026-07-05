package main

import (
	"errors"
	"testing"
)

func TestRecoverToResultCatchesPanic(t *testing.T) {
	result := recoverToResult(func() buildResult {
		panic("boom")
	})
	if !errors.Is(result.err, errEvalPanic) {
		t.Fatalf("err = %v, want errEvalPanic", result.err)
	}
}

func TestRecoverToResultPassesThrough(t *testing.T) {
	want := buildResult{diags: []Diagnostic{{Message: "x", Kind: kindIncomplete}}}
	result := recoverToResult(func() buildResult { return want })
	if len(result.diags) != 1 || result.diags[0].Message != "x" {
		t.Fatalf("diags = %+v, want pass-through of %+v", result.diags, want.diags)
	}
	if result.err != nil {
		t.Fatalf("err = %v, want nil", result.err)
	}
}
