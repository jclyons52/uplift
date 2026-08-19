package deps

import "sort"

// CouplingReport summarizes the coupling among a codebase's internal modules,
// derived from the module graph. Higher values = tighter coupling = harder to
// test/change in isolation; the point of the decoupling stage is to drive
// these down (fewer cycles, lower fan-out/density).
type CouplingReport struct {
	Modules       int     `json:"modules"`
	Edges         int     `json:"edges"`
	Density       float64 `json:"density"` // edges / (modules*(modules-1))
	AvgFanOut     float64 `json:"avgFanOut"`
	MaxFanOut     int     `json:"maxFanOut"`
	Hubs          []Hub   `json:"hubs"`   // top fan-out modules
	Cycles        int     `json:"cycles"` // distinct cycles found
	NodesInCycles int     `json:"nodesInCycles"`
}

// Hub is a module with high fan-out.
type Hub struct {
	Module string `json:"module"`
	FanOut int    `json:"fanOut"`
	FanIn  int    `json:"fanIn"`
}

// CouplingOf computes coupling metrics from an analyzed graph (internal nodes
// and their internal edges only — external imports are not module coupling).
func CouplingOf(r *Result) CouplingReport {
	var internal []string
	for _, name := range r.Order {
		if n := r.Nodes[name]; n != nil && n.Kind == KindInternal {
			internal = append(internal, name)
		}
	}
	inSet := map[string]bool{}
	for _, n := range internal {
		inSet[n] = true
	}

	// adjacency (internal -> internal)
	adj := map[string][]string{}
	edgeSet := map[string]bool{}
	edges := 0
	for _, name := range internal {
		n := r.Nodes[name]
		for _, dep := range n.Requires {
			if inSet[dep] {
				key := name + ">" + dep
				if !edgeSet[key] {
					edgeSet[key] = true
					edges++
				}
				adj[name] = append(adj[name], dep)
			}
		}
	}

	mod := len(internal)
	cr := CouplingReport{Modules: mod, Edges: edges}
	if mod > 1 {
		cr.Density = float64(edges) / float64(mod*(mod-1))
	}

	type fo struct {
		fanout int
		fanin  int
	}
	fos := map[string]fo{}
	totalFO := 0
	for _, name := range internal {
		fos[name] = fo{fanout: len(adj[name])}
		totalFO += len(adj[name])
	}
	for a := range adj {
		for _, b := range adj[a] {
			f := fos[b]
			f.fanin++
			fos[b] = f
		}
	}
	if mod > 0 {
		cr.AvgFanOut = float64(totalFO) / float64(mod)
	}
	var hubs []Hub
	for _, name := range internal {
		f := fos[name]
		if f.fanout > cr.MaxFanOut {
			cr.MaxFanOut = f.fanout
		}
		if f.fanout > 0 {
			hubs = append(hubs, Hub{Module: name, FanOut: f.fanout, FanIn: f.fanin})
		}
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].FanOut != hubs[j].FanOut {
			return hubs[i].FanOut > hubs[j].FanOut
		}
		return hubs[i].Module < hubs[j].Module
	})
	if len(hubs) > 10 {
		hubs = hubs[:10]
	}
	cr.Hubs = hubs

	// cycles = nodes that can reach themselves (Tarjan SCC)
	cycles, inCycles := tarjanCycles(internal, adj)
	cr.Cycles = cycles
	cr.NodesInCycles = inCycles
	return cr
}

// tarjanCycles runs Tarjan's SCC to find strongly-connected components with
// more than one node (or a self-loop), which are coupling cycles.
func tarjanCycles(nodes []string, adj map[string][]string) (cycleCount, nodesInCycles int) {
	idx := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	counter := 0
	var sccs [][]string
	var strongconnect func(v string)
	strongconnect = func(v string) {
		idx[v] = counter
		low[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if _, seen := idx[w]; !seen {
				strongconnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] {
				if idx[w] < low[v] {
					low[v] = idx[w]
				}
			}
		}
		if low[v] == idx[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}
	for _, n := range nodes {
		if _, seen := idx[n]; !seen {
			strongconnect(n)
		}
	}
	for _, scc := range sccs {
		if len(scc) > 1 {
			cycleCount++
			nodesInCycles += len(scc)
		}
	}
	return cycleCount, nodesInCycles
}
