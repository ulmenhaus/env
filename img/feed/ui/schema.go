package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StatusUnprocessed string = "Unprocessed"
	StatusPending     string = "Pending"
	StatusActive      string = "Active"
	StatusSatisfied   string = "Satisfied"

	FreshView     string = "Fresh"
	ResourcesView string = "Resources"

	FieldDescription string = "Description"
	FieldFeed        string = "Feed"
	FieldLink        string = "Link"
	FieldParent      string = "Parent"
	FieldStatus      string = "Status"

	TableNouns string = "nouns"

	JQLName string = "jql"
)
