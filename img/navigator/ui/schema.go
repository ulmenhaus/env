package ui

/*
Defines the constants of the code navigation schema for jql as well as the pm schema
*/

// TODO this is pretty common to any codedb tool so should be consolidated into a common library
const (
	JumpsTable     string = "jumps"
	ProjectsTable  string = "projects"
	BookmarksTable string = "bookmarks"

	ComponentsTable string = "components"

	ResourceView string = "Resources"
	TypeView     string = "Types"

	FieldDescription string = "Description"
	FieldOrder       string = "Order"
	FieldProject     string = "Project"
	FieldProjectName string = "A Name"
	FieldWorkdir     string = "Workdir"

	FieldDisplayName string = "DisplayName"
	FieldSrcLocation string = "SrcLocation"
)
