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

	ProjectsView string = "Projects"

	FieldBegin          string = "Begin"
	FieldDescription    string = "Auto Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldLink           string = "Link"
	FieldLogDescription string = "A Description"
	FieldProject        string = "Project"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"

	TableLog      string = "log"
	TableProjects string = "projects"
	TableTasks    string = "tasks"

	JQLName string = "jql"

	TasksView string = "tasks"
	LogView   string = "log"
)
