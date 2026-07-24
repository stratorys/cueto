// main composes the user's packages into one value at the module root, which is what
// inference reads (registries at the top level) and what cue vet checks as a whole.
package main

import (
	c "example.com/membrane/crew"
	p "example.com/membrane/components"
)

people:     c.people
components: p.components
