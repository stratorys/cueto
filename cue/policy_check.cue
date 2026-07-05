// Policy harness. Runs each opted-in pack against the concrete diagram and
// exposes the result as `policyReport`, a sibling of `diagram`. Because /eval
// marshals only `diagram`, this field never changes the eval output; Vet reads it
// and `policyAssert` gates `cue vet` in CI.
package diagram

import (
	"list"
	sec "github.com/stratorys/cueto/policy/security"
)

// policyReport is data, not an assertion: the Go backend reads it via /vet, so it
// must never make the package fail to build. The CI gate that turns a violation
// into a nonzero `cue vet` lives in a separate package (citool/) that imports
// this one, so it is invisible to the backend's load of `package diagram`.
policyReport: {
	if list.Contains(diagram.policies, "security") {
		security: (sec.#Pack & {d: diagram}).violations
	}
}
