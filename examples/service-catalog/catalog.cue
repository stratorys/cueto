// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The demo the README walks through: a small engineering organization as plain
// schema and data. No diagram is authored and nothing is imported from cueto.
// The graph is inferred from the registries (teams, people, services) and their
// key-set references (team, owner, techLead, dependsOn), and the knowledge
// runtime discovers the same registries plus the named evaluations below.
package main

import "list"

#TeamID: or([for id, _ in teams {id}])
#PersonID: or([for id, _ in people {id}])
#ServiceID: or([for id, _ in services {id}])

#Team: {
	name:    string
	channel: string
}

#Person: {
	name: string
	team: #TeamID
}

#Service: {
	name:     string
	owner:    #TeamID
	techLead: #PersonID
	tier:     "critical" | "standard" | "internal"
	dependsOn: [...#ServiceID]
}

teams: [ID=string]: #Team
teams: {
	platform: {name: "Platform", channel: "#team-platform"}
	payments: {name: "Payments", channel: "#team-payments"}
	web: {name: "Web", channel: "#team-web"}
}

people: [ID=string]: #Person
people: {
	alice: {name: "Alice Moreau", team: "platform"}
	bruno: {name: "Bruno Keller", team: "payments"}
	chloe: {name: "Chloé Diallo", team: "payments"}
	dana: {name: "Dana Costa", team: "web"}
}

services: [ID=string]: #Service
services: {
	gateway: {
		name:     "API Gateway"
		owner:    "platform"
		techLead: "alice"
		tier:     "critical"
	}
	billing: {
		name:     "Billing"
		owner:    "payments"
		techLead: "bruno"
		tier:     "critical"
		dependsOn: ["gateway", "ledger"]
	}
	ledger: {
		name:     "Ledger"
		owner:    "payments"
		techLead: "chloe"
		tier:     "critical"
	}
	storefront: {
		name:     "Storefront"
		owner:    "web"
		techLead: "dana"
		tier:     "standard"
		dependsOn: ["gateway", "billing"]
	}
}

evaluations: {
	ownerOf: {
		description: "Which team owns a service, and how to reach them"
		input: {serviceId: #ServiceID}
		result: {
			team:    services[input.serviceId].owner
			channel: teams[services[input.serviceId].owner].channel
			lead:    people[services[input.serviceId].techLead].name
		}
	}
	blastRadius: {
		description: "Which services break if this service goes down"
		input: {serviceId: #ServiceID}
		result: {
			dependents: [for id, s in services if list.Contains(s.dependsOn, input.serviceId) {id}]
		}
	}
}
