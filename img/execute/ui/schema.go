package ui

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusActive    string = "Active"
	StatusSatisfied  string = "Satisfied"
	StatusSomeday    string = "Someday"

	ProjectsView string = "Projects"

	FieldBegin          string = "Begin"
	FieldDescription    string = "Auto Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldLink           string = "Link"
	FieldLogDescription string = "A Description"
	FieldPrimaryGoal        string = "Primary Goal"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"

	TableLog      string = "log"
	TableProjects string = "projects"
	TableTasks    string = "tasks"

	JQLName string = "jql"

	TasksView string = "tasks"
	LogView   string = "log"
)
