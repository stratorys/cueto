# Architecture CI checks. `make check` validates the committed diagram against the
# schema and every opted-in policy pack (via the citool gate package); it exits
# nonzero on any violation, so it drops straight into a CI step.
.PHONY: check
check:
	cd cue && cue vet ./...
