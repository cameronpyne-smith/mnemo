package index

import "testing"

func TestFuseRRF(t *testing.T) {
	tests := []struct {
		name  string
		lists [][]string
		k     int
		limit int
		want  []Scored
	}{
		{
			name:  "doc in both lists beats doc topping one",
			lists: [][]string{{"a", "b", "c"}, {"c"}},
			k:     60,
			limit: 10,
			want: []Scored{
				{"c", 1.0/63 + 1.0/61},
				{"a", 1.0 / 61},
				{"b", 1.0 / 62},
			},
		},
		{
			name:  "mirrored ranks tie, broken by slug",
			lists: [][]string{{"a", "b", "c"}, {"b", "a", "d"}},
			k:     60,
			limit: 10,
			want: []Scored{
				{"a", 1.0/61 + 1.0/62},
				{"b", 1.0/61 + 1.0/62},
				{"c", 1.0 / 63},
				{"d", 1.0 / 63},
			},
		},
		{
			name:  "limit truncates",
			lists: [][]string{{"a", "b"}, {"a"}},
			k:     60,
			limit: 1,
			want:  []Scored{{"a", 1.0/61 + 1.0/61}},
		},
		{
			name:  "empty lists fuse to nothing",
			lists: [][]string{{}, {}},
			k:     60,
			limit: 10,
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FuseRRF(tt.lists, tt.k, tt.limit)
			if !scoredEqual(got, tt.want) {
				t.Errorf("FuseRRF = %v, want %v", got, tt.want)
			}
		})
	}
}
