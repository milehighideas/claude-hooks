package main

import (
	"fmt"
	"sort"
	"strings"
)

// verbToOp maps a curation verb to the tfplugingen-openapi config operation key
// and HTTP method. delete/read/update operate on the item path ({id}); create
// operates on the collection path.
type verbOp struct {
	op     string
	method string
	item   bool // true → path carries /{id}
}

var verbOps = map[string]verbOp{
	"create": {op: "create", method: "POST", item: false},
	"read":   {op: "read", method: "GET", item: true},
	"update": {op: "update", method: "PATCH", item: true},
	"delete": {op: "delete", method: "DELETE", item: true},
}

// emitOrder is the stable operation order in the generated config.
var emitOrder = []string{"create", "read", "update", "delete"}

// EmitGeneratorConfig renders the tfplugingen-openapi generator_config.yml for
// the given resolved resources. Resource keys are the singular table name;
// operations are emitted only for verbs present on each resource.
func EmitGeneratorConfig(rs []ResolvedResource) string {
	var b strings.Builder
	b.WriteString("provider:\n")
	b.WriteString("  name: dashtag\n")
	b.WriteString("resources:\n")

	sorted := make([]ResolvedResource, len(rs))
	copy(sorted, rs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Table < sorted[j].Table })

	for _, r := range sorted {
		key := singularize(r.Path)
		fmt.Fprintf(&b, "  %s:\n", key)
		base := "/api/v1/" + r.Path
		for _, verb := range emitOrder {
			if !hasVerb(r.Verbs, verb) {
				continue
			}
			vo := verbOps[verb]
			path := base
			if vo.item {
				path = base + "/{id}"
			}
			fmt.Fprintf(&b, "    %s:\n", vo.op)
			fmt.Fprintf(&b, "      path: %s\n", path)
			fmt.Fprintf(&b, "      method: %s\n", vo.method)
		}
	}
	return b.String()
}
