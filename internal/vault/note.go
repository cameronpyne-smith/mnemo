package vault

const (
	FolderNotes       = "notes"
	FolderHubs        = "hubs"
	FolderInbox       = "inbox"
	FolderAttachments = "attachments"
	FolderArchive     = "archive"
)

const (
	TypeNote = "note"
	TypeHub  = "hub"
)

type Frontmatter struct {
	Description string         `yaml:"description"`
	Tags        []string       `yaml:"tags,omitempty"`
	Type        string         `yaml:"type,omitempty"`
	Created     string         `yaml:"created,omitempty"`
	Updated     string         `yaml:"updated,omitempty"`
	Extra       map[string]any `yaml:",inline"`
}

type Note struct {
	Slug        string
	Folder      string
	Frontmatter Frontmatter
	Body        string
}

func (n *Note) Type() string {
	if n.Frontmatter.Type == "" {
		return TypeNote
	}
	return n.Frontmatter.Type
}

func (n *Note) Links() []string {
	return ExtractWikilinks(n.Body)
}
