// Broken variant, Layer 1: backend.owner names "alice", who has been removed from
// crew. The typed reference no longer resolves, so `cue vet` (cueto vet) rejects the
// module. Every readme file here exists; this fixture isolates the referential-
// integrity failure the compiler decides on its own.
package components

import "example.com/membrane/crew"

#Component: {
	name:   string
	owner:  crew.#PersonKey
	readme: string @file()
	deps: [...#ComponentKey]
}

components: [ID=string]: #Component
components: {
	backend: {name: "Backend", owner: "alice", readme: "docs/backend.md", deps: ["frontend"]}
	frontend: {name: "Frontend", owner: "bob", readme: "docs/frontend.md", deps: []}
}

#ComponentKey: or([for k, _ in components {k}])
