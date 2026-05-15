package embed

import (
	"math"
	"testing"

	"github.com/rdegges/redline/internal/store"
)

func TestCosine_Symmetric(t *testing.T) {
	a := []float32{0.1, 0.2, 0.3}
	b := []float32{0.4, 0.5, 0.6}
	s1 := Cosine(a, b)
	s2 := Cosine(b, a)
	if math.Abs(s1-s2) > 1e-9 {
		t.Fatalf("not symmetric: %v vs %v", s1, s2)
	}
}

func TestCosine_SelfIsOne(t *testing.T) {
	v := []float32{1, 2, 3, 4, 5}
	if math.Abs(Cosine(v, v)-1.0) > 1e-6 {
		t.Fatalf("Cosine(v,v) != 1")
	}
}

func TestFindPairs_Threshold(t *testing.T) {
	emb := []store.Embedding{
		{PageURL: "http://a", Vector: []float32{1, 0, 0}},
		{PageURL: "http://b", Vector: []float32{1, 0.1, 0}},
		{PageURL: "http://c", Vector: []float32{0, 1, 0}},
	}
	pairs := FindPairs(emb, 0.9)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d: %+v", len(pairs), pairs)
	}
	if pairs[0].A != "http://a" || pairs[0].B != "http://b" {
		t.Fatalf("wrong pair: %+v", pairs[0])
	}
}

func TestAssignClusters_DeterministicIDs(t *testing.T) {
	pairs := []Pair{
		{A: "z", B: "y", Score: 0.95},
		{A: "y", B: "x", Score: 0.95},
		{A: "c", B: "b", Score: 0.95},
	}
	clusters := AssignClusters(pairs)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	// Cluster IDs assigned by lex-smallest member.
	if clusters[0].ID != "cl_001" || clusters[1].ID != "cl_002" {
		t.Fatalf("ids: %v %v", clusters[0].ID, clusters[1].ID)
	}
	// Smallest member of cluster 1 is "b", cluster 2 is "x".
	if clusters[0].Members[0] != "b" || clusters[1].Members[0] != "x" {
		t.Fatalf("members: %+v", clusters)
	}
}

func TestCanonicalPage_HighestWordCountWins(t *testing.T) {
	cluster := Cluster{Members: []string{"a", "b"}}
	pages := map[string]store.Page{
		"a": {URL: "a", WordCount: 100},
		"b": {URL: "b", WordCount: 200},
	}
	if got := CanonicalPage(cluster, pages); got != "b" {
		t.Fatalf("got %q want b", got)
	}
}
