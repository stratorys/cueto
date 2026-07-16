// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package knowledge

import "cuelang.org/go/cue"

// EvaluationProjection discovers an optional, CUE-native knowledge.evaluations
// namespace. Values are intentionally retained as CUE values for later typed
// query/export adapters.
type EvaluationProjection struct{}

func (EvaluationProjection) Name() string { return "evaluations" }

func (EvaluationProjection) Discover(value cue.Value) (any, error) {
	return value.LookupPath(cue.ParsePath("knowledge.evaluations")), nil
}
