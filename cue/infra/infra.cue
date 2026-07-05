// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Normalized topology that every importer (docker-compose first, then k8s,
// terraform, openapi) emits. The drift harness compares a diagram against an
// #Actual value overlaid by the backend during /vet.
package infra

#Actual: {
	source: "compose" | "k8s" | "terraform" | "openapi"
	services: [Name=string]: {
		name:    Name
		image?:  string
		region?: string
		zone?:   string
	}
	links: [...#Link]
}

// A dependency/traffic edge in the live topology: source uses/depends-on target,
// matching the diagram's source->target convention.
#Link: {
	source: string
	target: string
	kind?:  string
}
