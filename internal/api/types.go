package api

type SearchResult struct {
	Slug        string  `json:"slug"`
	Folder      string  `json:"folder"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type Note struct {
	Slug        string   `json:"slug"`
	Folder      string   `json:"folder"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Body        string   `json:"body"`
	Links       []string `json:"links,omitempty"`
	Backlinks   []string `json:"backlinks,omitempty"`
}

type IndexResponse struct {
	Root Note   `json:"root"`
	Hubs []Note `json:"hubs"`
}

type CaptureRequest struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

type CaptureResponse struct {
	Slug string `json:"slug"`
}

type EditRequest struct {
	Description *string   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Body        *string   `json:"body,omitempty"`
	Append      *string   `json:"append,omitempty"`
}

type RenameRequest struct {
	To string `json:"to"`
}

type LinksResponse struct {
	Slug      string   `json:"slug"`
	Links     []string `json:"links"`
	Backlinks []string `json:"backlinks"`
}

type FilingStatus struct {
	Enabled   bool `json:"enabled"`
	Processed int  `json:"processed"`
	Failed    int  `json:"failed"`
	InFlight  int  `json:"in_flight"`
}

type StatusResponse struct {
	Notes    int          `json:"notes"`
	Hubs     int          `json:"hubs"`
	Inbox    int          `json:"inbox"`
	Archived int          `json:"archived"`
	Filing   FilingStatus `json:"filing"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
