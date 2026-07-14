package vault

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func NewCapture(content, source string, now time.Time) *Note {
	suffix := make([]byte, 2)
	rand.Read(suffix)
	date := now.Format("2006-01-02")
	desc := "Raw capture awaiting filing."
	if source != "" {
		desc = fmt.Sprintf("Raw capture from %s awaiting filing.", source)
	}
	return &Note{
		Slug:   fmt.Sprintf("capture-%s-%s", now.Format("20060102-150405"), hex.EncodeToString(suffix)),
		Folder: FolderInbox,
		Frontmatter: Frontmatter{
			Description: desc,
			Created:     date,
			Updated:     date,
			Extra:       map[string]any{"source": source},
		},
		Body: strings.TrimRight(content, "\n") + "\n",
	}
}
