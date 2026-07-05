// CI gate package. Separate from `package diagram` on purpose: `cue vet ./...`
// evaluates every package in the module, so this assertion runs in CI; but the
// backend loads only the root `.` (package diagram), so this file never affects
// /eval or /vet. Unifying a nonzero violation count with 0 is an error, making
// `cue vet ./...` exit nonzero when the committed diagram violates an opted-in
// policy pack.
package citool

import d "github.com/stratorys/cue-diagram:diagram"

gate: {
	for pack, violations in d.policyReport {
		(pack): len(violations) & 0
	}
}
