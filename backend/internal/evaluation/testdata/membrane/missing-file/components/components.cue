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
	// readme points at a file that does not exist: valid CUE, failed graph check.
	frontend: {name: "Frontend", owner: "bob", readme: "docs/missing.md", deps: []}
}

#ComponentKey: or([for k, _ in components {k}])
