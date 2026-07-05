package diagram

diagram: #Diagram & {
	nodes: {
		user: {
			type: "table"
			x: 80
			y: 80
			label: "user"
			columns: [
				{
					name: "id"
					dbType: "uuid"
					pk: true
				},
				{
					name: "email"
					dbType: "text"
				},
			]
		}
		order: {
			type: "table"
			x: 460
			y: 120
			label: "order"
			columns: [
				{
					name: "id"
					dbType: "uuid"
					pk: true
				},
				{
					name: "user_id"
					dbType: "uuid"
					fk: true
				},
				{
					name: "total"
					dbType: "numeric"
				},
			]
		}
		review: {
			type: "process"
			x: 300
			y: 380
			label: "review order"
		}
	}
	edges: [
		{
			id: "e_user_order"
			source: "user"
			sourceHandle: "id-source"
			target: "order"
			targetHandle: "user_id-target"
			kind: "relation"
			card: "1-n"
		},
		{
			id: "e_order_review"
			source: "order"
			sourceHandle: "id-source"
			target: "review"
			kind: "arrow"
		},
	]
}
