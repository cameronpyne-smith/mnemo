package vault

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	slugRe        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonSlugCharRe = regexp.MustCompile(`[^a-z0-9]+`)
)

func ValidSlug(s string) bool {
	return slugRe.MatchString(s)
}

func Slugify(title string) (string, error) {
	s := strings.ToLower(title)
	s = nonSlugCharRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "", fmt.Errorf("cannot derive slug from %q", title)
	}
	return s, nil
}
