package index

import (
	"math"
	"testing"
)

func scoredEqual(a, b []Scored) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Slug != b[i].Slug || math.Abs(a[i].Score-b[i].Score) > 1e-6 {
			return false
		}
	}
	return true
}

func TestTopK(t *testing.T) {
	docs := []DocVector{
		{Slug: "north", Vec: []float32{0, 1}},
		{Slug: "east", Vec: []float32{1, 0}},
		{Slug: "northeast", Vec: []float32{0.6, 0.8}},
		{Slug: "south", Vec: []float32{0, -1}},
	}
	tests := []struct {
		name  string
		query []float32
		docs  []DocVector
		k     int
		want  []Scored
	}{
		{
			name:  "ranks by cosine descending",
			query: []float32{0, 1},
			docs:  docs,
			k:     3,
			want:  []Scored{{"north", 1}, {"northeast", 0.8}, {"east", 0}},
		},
		{
			name:  "k beyond corpus returns all, negatives included and last",
			query: []float32{0, 1},
			docs:  docs,
			k:     10,
			want:  []Scored{{"north", 1}, {"northeast", 0.8}, {"east", 0}, {"south", -1}},
		},
		{
			name:  "equal scores tie-break by slug",
			query: []float32{1, 0},
			docs:  docs,
			k:     10,
			want:  []Scored{{"east", 1}, {"northeast", 0.6}, {"north", 0}, {"south", 0}},
		},
		{
			name:  "k of zero returns nothing",
			query: []float32{0, 1},
			docs:  docs,
			k:     0,
			want:  nil,
		},
		{
			name:  "empty corpus returns nothing",
			query: []float32{0, 1},
			docs:  nil,
			k:     5,
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TopK(tt.query, tt.docs, tt.k)
			if !scoredEqual(got, tt.want) {
				t.Errorf("TopK = %v, want %v", got, tt.want)
			}
		})
	}
}
