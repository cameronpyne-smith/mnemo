package vault

import (
	"fmt"
	"os"
	"regexp"
)

func RewriteLinks(body, oldSlug, newSlug string) string {
	re := regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(oldSlug) + `\s*([|#][^\]]*)?\]\]`)
	return re.ReplaceAllString(body, "[["+newSlug+"$1]]")
}

func (v *Vault) Delete(folder, slug string) error {
	if err := os.Remove(v.NotePath(folder, slug)); err != nil {
		return fmt.Errorf("deleting note %s/%s: %w", folder, slug, err)
	}
	return nil
}
