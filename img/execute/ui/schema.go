package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusActive     string = "Active"
	StatusSatisfied  string = "Satisfied"
	StatusSomeday    string = "Someday"

	ProjectsView string = "Projects"

	FieldAction         string = "Action"
	FieldBegin          string = "Begin"
	FieldDescription    string = "_Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldIndirect       string = "Indirect"
	FieldLink           string = "Link"
	FieldLogDescription string = "A_Description"
	FieldPrimaryGoal    string = "Primary Goal"
	FieldSpan           string = "Param~Span"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"

	TableLog      string = "log"
	TableTasks    string = "tasks"

	JQLName string = "jql"

	CountsView string = "counts"
	TasksView string = "tasks"
	LogView   string = "log"

	SpanDay   string = "Day"
	SpanWeek  string = "Week"
	SpanMonth string = "Month"
)

var Spans []string = []string{SpanDay, SpanWeek, SpanMonth}
