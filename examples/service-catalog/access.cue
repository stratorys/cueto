// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Who may do what. Each role is a disjunction (a set) of permissions, so roles
// form a semilattice under unification: `roles.a & roles.b` is the intersection
// of what both allow, `_` (top) allows anything, and two disjoint permissions
// unify to bottom. The REPL section of the README walks through it.
package main

#Perm: "read" | "write" | "deploy" | "admin"

roles: {
	viewer:    "read"
	developer: "read" | "write"
	operator:  "read" | "write" | "deploy"
	owner:     #Perm
}
