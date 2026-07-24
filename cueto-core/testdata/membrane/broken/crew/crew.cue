// Broken variant: alice has been removed from the roster. Every fact still naming
// her must now fail the build - that is the membrane working.
package crew

#Person: {
	name: string
}

people: [ID=string]: #Person
people: {
	bob: {name: "Bob"}
}

#PersonKey: or([for k, _ in people {k}])
