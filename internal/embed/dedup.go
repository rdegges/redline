package embed

import (
	"fmt"
	"sort"

	"github.com/rdegges/redline/internal/store"
)

// Pair is a pair of pages with above-threshold cosine similarity.
type Pair struct {
	A, B  string // URLs, ordered so A < B lexicographically
	Score float64
}

// Cluster groups pages connected by Pair edges.
type Cluster struct {
	ID      string
	Members []string
}

// FindPairs computes all pairwise cosine similarities and returns those
// at or above threshold, ordered for determinism.
func FindPairs(embeddings []store.Embedding, threshold float64) []Pair {
	// Sort embeddings by URL for deterministic iteration.
	sort.Slice(embeddings, func(i, j int) bool { return embeddings[i].PageURL < embeddings[j].PageURL })
	var out []Pair
	for i := 0; i < len(embeddings); i++ {
		for j := i + 1; j < len(embeddings); j++ {
			s := Cosine(embeddings[i].Vector, embeddings[j].Vector)
			if s < threshold {
				continue
			}
			a, b := embeddings[i].PageURL, embeddings[j].PageURL
			if a > b {
				a, b = b, a
			}
			out = append(out, Pair{A: a, B: b, Score: s})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}

// AssignClusters takes the Pair list and groups members via union-find.
// Cluster IDs are assigned in order of each cluster's lexicographically
// smallest URL.
func AssignClusters(pairs []Pair) []Cluster {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		p, ok := parent[x]
		if !ok {
			parent[x] = x
			return x
		}
		if p == x {
			return x
		}
		r := find(p)
		parent[x] = r
		return r
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	for _, p := range pairs {
		union(p.A, p.B)
	}
	groups := map[string][]string{}
	for k := range parent {
		root := find(k)
		groups[root] = append(groups[root], k)
	}
	// Determine root order by lex-smallest member.
	roots := make([]string, 0, len(groups))
	for r := range groups {
		sort.Strings(groups[r])
		roots = append(roots, r)
	}
	sort.Strings(roots)
	clusters := make([]Cluster, 0, len(roots))
	for i, r := range roots {
		clusters = append(clusters, Cluster{
			ID:      fmt.Sprintf("cl_%03d", i+1),
			Members: groups[r],
		})
	}
	return clusters
}

// CanonicalPage chooses the page-to-keep from a cluster, using these
// tie-breakers: highest word_count, then earliest last_modified, then
// earliest discovered_at, then lex-lowest URL.
func CanonicalPage(cluster Cluster, pageByURL map[string]store.Page) string {
	if len(cluster.Members) == 0 {
		return ""
	}
	best := cluster.Members[0]
	for _, m := range cluster.Members[1:] {
		pb, ok1 := pageByURL[best]
		pm, ok2 := pageByURL[m]
		if !ok1 && ok2 {
			best = m
			continue
		}
		if !ok2 {
			continue
		}
		if pm.WordCount != pb.WordCount {
			if pm.WordCount > pb.WordCount {
				best = m
			}
			continue
		}
		// last_modified ascending wins (older = original).
		if pm.LastModified.String < pb.LastModified.String && pm.LastModified.String != "" {
			best = m
			continue
		}
		if m < best {
			best = m
		}
	}
	return best
}
