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

	FieldDescription string = "Description"
	FieldFeed        string = "Feed"
	FieldLink        string = "Link"
	FieldResource    string = "Resource"
	FieldStatus      string = "Status"

	TableResources string = "resources"
	TableItems     string = "items"

	JQLName string = "jql"
)
