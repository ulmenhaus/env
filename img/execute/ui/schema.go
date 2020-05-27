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

	FieldAction          string = "Action"
	FieldBegin          string = "Begin"
	FieldDescription    string = " Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldIndirect       string = "Indirect"
	FieldLink           string = "Link"
	FieldLogDescription string = "A Description"
	FieldPrimaryGoal    string = "Primary Goal"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"

	TableLog      string = "log"
	TableProjects string = "projects"
	TableTasks    string = "tasks"

	JQLName string = "jql"

	TasksView string = "tasks"
	LogView   string = "log"

	HabitRegularity     string = "regularity"
	HabitIncrementality string = "incrementality"
	HabitBreakdown      string = "breakdown"

	ActionExtend string = "Extend"
	ActionImprove string = "Improve"
	ActionSustain string = "Sustain"
)
