package vault

import (
	"reflect"
	"testing"
)

func TestExtractWikilinks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"plain link", "See [[birth-plan]].", []string{"birth-plan"}},
		{"alias", "See [[birth-plan|the plan]].", []string{"birth-plan"}},
		{"heading anchor", "See [[birth-plan#risks]].", []string{"birth-plan"}},
		{"alias and heading", "See [[birth-plan#risks|risks]].", []string{"birth-plan"}},
		{"multiple and duplicate", "[[a]] then [[b]] then [[a]] again.", []string{"a", "b"}},
		{"whitespace trimmed", "[[ spaced-link ]]", []string{"spaced-link"}},
		{"no links", "Nothing here.", nil},
		{"empty target ignored", "[[|alias only]]", nil},
		{"not nested brackets", "[[outer]] and [not-a-link]", []string{"outer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractWikilinks(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractWikilinks(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
