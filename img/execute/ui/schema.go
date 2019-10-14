package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusSomeday    string = "Someday"
	StatusPending    string = "Pending"
	StatusSatisfied  string = "Satisfied"

	ResourcesView string = "Resources"

	FieldBegin          string = "Begin"
	FieldDescription    string = "Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldItem           string = "Item"
	FieldLink           string = "Link"
	FieldLogDescription string = "A Description"
	FieldResource       string = "Resource"
	FieldStatus         string = "Status"

	TableResources string = "resources"
	TableItems     string = "items"
	TableLog       string = "log"

	JQLName string = "jql"

	ItemsView string = "items"
	LogView   string = "log"
)
