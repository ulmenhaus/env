package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StatusIdea         string = "Idea"
	StatusExploring    string = "Exploring"
	StatusPlanning     string = "Planning"
	StatusImplementing string = "Implementing"
	StatusSatisfied    string = "Satisfied"
	StatusFresh        string = "Fresh" // Fresh is not a real status in the schma, but an internal way to denote potential ideas from automted feeds

	ResourcesView string = "Resources"
	DomainView    string = "Domains"
	StatsView     string = "Status"
	NewTaskView   string = "new_task"

	Stage1View string = "Stage1"
	Stage2View string = "Stage2"
	Stage3View string = "Stage3"
	Stage4View string = "Stage4"

	FieldIdentifier  string = "_Identifier"
	FieldDescription string = "Description"
	FieldContext     string = "Context"
	FieldCoordinal   string = "Coordinal"
	FieldCode        string = "Code"
	FieldFeed        string = "Feed"
	FieldLink        string = "Link"
	FieldParent      string = "Parent"
	FieldStatus      string = "Status"
	FieldRelation    string = "A Relation"
	FieldArg0        string = "Arg0"
	FieldArg1        string = "Arg1"
	FieldOrder       string = "Order"
	FieldModifier    string = "A Modifier"

	TableNouns      string = "nouns"
	TableAssertions string = "assertions"
	TableContexts   string = "contexts"

	JQLName string = "jql"

	FeedManual string = "manual"

	ValueProjectPlans string = "project plans"
)
