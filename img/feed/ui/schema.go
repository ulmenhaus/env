package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StatusIdea      string = "Idea"
	StatusPending   string = "Pending"
	StatusActive    string = "Active"
	StatusSatisfied string = "Satisfied"

	ResourcesView string = "Resources"
	FreshView     string = "Fresh"
	DomainView    string = "Domains"

	FieldDescription string = "Description"
	FieldFeed        string = "Feed"
	FieldLink        string = "Link"
	FieldParent      string = "Parent"
	FieldStatus      string = "Status"
	FieldRelation    string = "A Relation"
	FieldArg0 string = "Arg0"
	FieldArg1 string = "Arg1"

	TableNouns      string = "nouns"
	TableAssertions string = "assertions"

	JQLName string = "jql"
)
