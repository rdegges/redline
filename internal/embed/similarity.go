// Package embed implements Pass 2 — embedding-based duplicate detection.
package embed

import "math"

// Cosine returns the cosine similarity between two equal-length vectors.
// Returns 0 if either vector has zero magnitude. +
// (cos(v,v)==1, |cos|≤1, symmetric).
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
