package vault

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNote(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   Frontmatter
		wantBody string
	}{
		{
			name:     "full frontmatter",
			content:  "---\ndescription: A test note.\ntags: [a, b]\ntype: hub\ncreated: 2026-07-14\n---\n\nBody here.\n",
			wantFM:   Frontmatter{Description: "A test note.", Tags: []string{"a", "b"}, Type: "hub", Created: "2026-07-14"},
			wantBody: "Body here.\n",
		},
		{
			name:     "no frontmatter",
			content:  "Just a body.\n",
			wantFM:   Frontmatter{},
			wantBody: "Just a body.\n",
		},
		{
			name:     "crlf line endings",
			content:  "---\r\ndescription: Windows note.\r\n---\r\n\r\nBody.\r\n",
			wantFM:   Frontmatter{Description: "Windows note."},
			wantBody: "Body.\n",
		},
		{
			name:     "unknown fields preserved in extra",
			content:  "---\ndescription: Foreign note.\ncustom-field: kept\n---\n\nBody.\n",
			wantFM:   Frontmatter{Description: "Foreign note.", Extra: map[string]any{"custom-field": "kept"}},
			wantBody: "Body.\n",
		},
		{
			name:     "frontmatter only, no body",
			content:  "---\ndescription: Empty body.\n---",
			wantFM:   Frontmatter{Description: "Empty body."},
			wantBody: "",
		},
		{
			name:     "unterminated frontmatter treated as body",
			content:  "---\ndescription: broken\n\nText.\n",
			wantFM:   Frontmatter{},
			wantBody: "---\ndescription: broken\n\nText.\n",
		},
		{
			name:     "horizontal rule later in body without frontmatter",
			content:  "Intro.\n\n---\n\nOutro.\n",
			wantFM:   Frontmatter{},
			wantBody: "Intro.\n\n---\n\nOutro.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := ParseNote([]byte(tt.content))
			if err != nil {
				t.Fatalf("ParseNote: %v", err)
			}
			if !reflect.DeepEqual(fm, tt.wantFM) {
				t.Errorf("frontmatter = %+v, want %+v", fm, tt.wantFM)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestEncodeNoteRoundTrip(t *testing.T) {
	fm := Frontmatter{
		Description: "Round trip.",
		Tags:        []string{"x"},
		Created:     "2026-07-14",
		Extra:       map[string]any{"foreign": "value"},
	}
	body := "# Title\n\nSome text with a [[link]].\n"

	encoded, err := EncodeNote(fm, body)
	if err != nil {
		t.Fatalf("EncodeNote: %v", err)
	}
	gotFM, gotBody, err := ParseNote(encoded)
	if err != nil {
		t.Fatalf("ParseNote: %v", err)
	}
	if !reflect.DeepEqual(gotFM, fm) {
		t.Errorf("frontmatter = %+v, want %+v", gotFM, fm)
	}
	if gotBody != body {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
}

func TestEncodeNoteAddsTrailingNewline(t *testing.T) {
	encoded, err := EncodeNote(Frontmatter{Description: "d"}, "no newline")
	if err != nil {
		t.Fatalf("EncodeNote: %v", err)
	}
	if !strings.HasSuffix(string(encoded), "no newline\n") {
		t.Errorf("encoded note missing trailing newline: %q", encoded)
	}
}
