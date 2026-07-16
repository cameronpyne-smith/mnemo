package vault

import "testing"

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
