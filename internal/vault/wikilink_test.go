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
		{"fenced code ignored", "```bash\nif [[ -n \"$win_home\" && -f \"$win_home/.kube/config\" ]]; then\nfi\n```\nSee [[real]].", []string{"real"}},
		{"fence with language and indent", "  ```go\n  x := m[[2]string{\"a\", \"b\"}]\n  ```\n[[real]]", []string{"real"}},
		{"tilde fence ignored", "~~~\n[[not-a-link]]\n~~~\n[[real]]", []string{"real"}},
		{"unclosed fence swallows rest", "before [[real]]\n```\n[[not-a-link]]", []string{"real"}},
		{"inline code ignored", "run `kubectl [[pod]]` then see [[real]]", []string{"real"}},
		{"backtick fence not closed by tilde", "```\n~~~\n[[not-a-link]]\n```\n[[real]]", []string{"real"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractWikilinks(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractWikilinks(%q) = %q, want %v", tt.body, got, tt.want)
			}
		})
	}
}
