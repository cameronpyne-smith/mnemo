package vault

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	validSlugRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonSlugCharRe = regexp.MustCompile(`[^a-z0-9]+`)
)

func ValidSlug(s string) bool {
	return validSlugRe.MatchString(s)
}

func Slugify(title string) (string, error) {
	lower := strings.ToLower(title)
	kebabed := nonSlugCharRe.ReplaceAllString(lower, "-")
	result := strings.Trim(kebabed, "-")

	if !ValidSlug(result) {
		return "", fmt.Errorf("cannot derive slug from %q", title)
	}

	return result, nil
}
