package store

import "fmt"

// WalkRelation does a breadth-first traversal of edges of the given relation,
// outward from startID, up to maxDepth hops (agentic-lbjg, ADR-016). It returns
// every reachable node exactly once (node-ID visited set), each carrying its
// minimum (BFS) depth, in BFS order with the start node first at depth 0.
//
// Direction: outgoing=true walks source->target (an edge counts when its
// SourceID is the frontier node, and the TargetID is the neighbour); outgoing=
// false walks target->source (the mirror). Cycles and self-loops terminate on
// the visited set — the start node is pre-seeded, so a self-loop A->A never
// re-adds A. maxDepth<=0 returns just the start node ([start@0]). An unknown
// start node is an error (matching GetNode's contract).
//
// This is the reusable multi-hop primitive: agentic-sx4u inherits it. It does
// NOT author new SQL — it reuses GetEdgesBatch (which is source-OR-target, so
// direction is filtered here in Go) and passes asOf=nil, so the validity
// predicate is omitted (the "as-of provenance" walk is a documented
// second-iteration feature, out of scope for v1).
//
// BFS-in-Go is chosen over a recursive CTE because a `WITH RECURSIVE ... UNION`
// dedupes by (node, depth) row, not by node, so it re-walks a cycle to the depth
// cap; a node-keyed visited set gives clean per-node-once semantics. See ADR-016
// for the live cycle-over-walk probe evidence.
func (s *Store) WalkRelation(startID, relation string, maxDepth int, outgoing bool) ([]NodeWithDepth, error) {
	start, err := s.GetNode(startID)
	if err != nil {
		return nil, err
	}

	result := []NodeWithDepth{{Node: *start, Depth: 0}}
	visited := map[string]bool{startID: true}

	if maxDepth <= 0 {
		return result, nil
	}

	frontier := []string{startID}
	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		// One batched edge fetch per depth level (asOf=nil → no validity filter).
		edgeMap, err := s.GetEdgesBatch(frontier, nil)
		if err != nil {
			return nil, fmt.Errorf("walking %s from %s at depth %d: %w", relation, startID, depth, err)
		}

		var next []string
		for _, node := range frontier {
			for i := range edgeMap[node] {
				e := &edgeMap[node][i]
				if e.Relation != relation {
					continue
				}
				// Select only edges oriented per the requested direction. The
				// frontier node must be on the correct end of the edge.
				var neighbour string
				if outgoing {
					if e.SourceID != node {
						continue
					}
					neighbour = e.TargetID
				} else {
					if e.TargetID != node {
						continue
					}
					neighbour = e.SourceID
				}

				if visited[neighbour] {
					continue
				}
				visited[neighbour] = true

				n, err := s.GetNode(neighbour)
				if err != nil {
					// An edge can reference a node that is no longer active /
					// present; skip it rather than fail the whole walk.
					continue
				}
				result = append(result, NodeWithDepth{Node: *n, Depth: depth + 1})
				next = append(next, neighbour)
			}
		}
		frontier = next
	}

	return result, nil
}
