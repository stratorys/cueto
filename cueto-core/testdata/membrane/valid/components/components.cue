// components is example vocabulary authored by the user. Each component names an
// owner (a key-set reference into crew) and a readme file that must exist on disk
// (the @file graph check).
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
