// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"errors"
	"testing"

	"github.com/stratorys/cueto/backend/internal/diag"
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
	want := buildResult{diags: []diag.Diagnostic{{Message: "x", Kind: diag.KindIncomplete}}}
	result := recoverToResult(func() buildResult { return want })
	if len(result.diags) != 1 || result.diags[0].Message != "x" {
		t.Fatalf("diags = %+v, want pass-through of %+v", result.diags, want.diags)
	}
	if result.err != nil {
		t.Fatalf("err = %v, want nil", result.err)
	}
}
