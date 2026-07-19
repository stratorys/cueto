// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// How services run in each environment. Configs form a meet-semilattice under
// unification: every concrete environment is `configBase & overlay`, the
// greatest lower bound of both, defaults fill what the overlay leaves open, and
// an overlay that contradicts the base is bottom, a build error.
package main

#Config: {
	replicas: int & >=1
	logLevel: "debug" | "info" | "error"
	memoryMb: int & >=128
}

configBase: #Config & {
	replicas: *1 | _
	logLevel: *"info" | _
	memoryMb: *256 | _
}

environments: [ID=string]: #Config
environments: {
	dev: configBase
	staging: configBase & {replicas: 2}
	prod: configBase & {replicas: 3, logLevel: "error", memoryMb: 1024}
}
