// crew is example vocabulary authored by the user, not shipped by cueto. It is a
// registry of people whose keys become the valid owner set for other packages.
package crew

#Person: {
	name: string
}

people: [ID=string]: #Person
people: {
	alice: {name: "Alice"}
	bob: {name: "Bob"}
}

// #PersonKey is the membrane idiom: the set of keys that actually exist. A field
// constrained to it can only name a person really declared here, and cueto reads it
// as a relation for inference.
#PersonKey: or([for k, _ in people {k}])
