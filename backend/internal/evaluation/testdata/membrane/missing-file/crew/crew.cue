// missing-file variant, Layer 2: the CUE is fully valid (every owner exists), so
// `cueto vet` passes. One component's readme names a file that is not on disk, which
// only `cueto check` can catch - the compiler cannot see the filesystem.
package crew

#Person: {
	name: string
}

people: [ID=string]: #Person
people: {
	alice: {name: "Alice"}
	bob: {name: "Bob"}
}

#PersonKey: or([for k, _ in people {k}])
