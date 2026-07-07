# Architecture CI checks. `make check` validates the committed module: Layer 1 is
# pure-CUE validity (`cue vet` and `cueto vet`), Layer 2 is the world-facing graph
# checks the compiler cannot do (`cueto check` - @file/@uri references resolve). It
# exits nonzero on any violation, so it drops straight into a CI step.
.PHONY: check
check:
	cd cue && cue vet ./...
	cd backend && go run ./cmd/cueto vet -C ../cue
	cd backend && go run ./cmd/cueto check -C ../cue
