package ui

/*
Defines the constants of the code navigation schema for jql as well as the pm schema
*/

// TODO this is pretty common to any codedb tool so should be consolidated into a common library
const (
	ResourceView string = "Resources"
	TypeView     string = "Types"
)

type Bookmark struct{}

type Project struct {
	Workdir   string              `json:"workdir"`
	Bookmarks map[string]Bookmark `json:"bookmarks"`
	Jumps     []string            `json:"jumps"`
}

type ProjectsFile struct {
	Projects map[string]Project `json:"projects"`
}
