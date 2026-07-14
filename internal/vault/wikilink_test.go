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

func TestSlugify(t *testing.T) {
	tests := []struct {
		title   string
		want    string
		wantErr bool
	}{
		{"Birth Plan", "birth-plan", false},
		{"  Hello,   World!  ", "hello-world", false},
		{"already-a-slug", "already-a-slug", false},
		{"Ünïcode Note", "n-code-note", false},
		{"!!!", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got, err := Slugify(tt.title)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Slugify(%q) error = %v, wantErr %v", tt.title, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.title, got, tt.want)
			}
			if got != "" && !ValidSlug(got) {
				t.Errorf("Slugify(%q) = %q fails ValidSlug", tt.title, got)
			}
		})
	}
}
