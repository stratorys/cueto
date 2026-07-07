// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"go/build"
	"testing"
)

// TestEvaluationDoesNotImportSiblingConcerns enforces the dependency rule: the
// pure engine must not import the persistence (repo) or content (source formats)
// concerns. A violation means a disk or source-format dependency leaked into
// evaluation, breaking the transports -> services -> domain layering.
func TestEvaluationDoesNotImportSiblingConcerns(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("import evaluation package: %v", err)
	}
	forbidden := map[string]bool{
		"github.com/stratorys/cueto/backend/internal/repo":    true,
		"github.com/stratorys/cueto/backend/internal/content": true,
	}
	for _, imp := range pkg.Imports {
		if forbidden[imp] {
			t.Errorf("evaluation imports %q, which the dependency rule forbids", imp)
		}
	}
}
