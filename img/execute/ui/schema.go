package ui

import "github.com/ulmenhaus/env/img/jql/types"

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusPending    string = "Pending"
	StatusActive     string = "Active"
	StatusSatisfied  string = "Satisfied"
	StatusSomeday    string = "Someday"

	ProjectsView string = "Projects"

	FieldAction         string = "Action"
	FieldBegin          string = "Begin"
	FieldDescription    string = "_Description"
	FieldEnd            string = "End"
	FieldFeed           string = "Feed"
	FieldDirect         string = "Direct"
	FieldIndirect       string = "Indirect"
	FieldLink           string = "Link"
	FieldLogDescription string = "A_Description"
	FieldPrimaryGoal    string = "Primary Goal"
	FieldSpan           string = "Param~Span"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"

	TableLog   string = "log"
	TableTasks string = "tasks"

	JQLName string = "jql"

	CountsView string = "counts"
	TasksView  string = "tasks"
	LogView    string = "log"

	SpanDay     string = "Day"
	SpanWeek    string = "Week"
	SpanMonth   string = "Month"
	SpanQuarter string = "Quarter"
	SpanPending string = "Pending"
)

var Spans []string = []string{SpanDay, SpanWeek, SpanMonth, SpanPending}

var Span2Title map[string]string = map[string]string{
	SpanDay:     "Today",
	SpanWeek:    "This Week",
	SpanMonth:   "This Month",
	SpanPending: "Pending",
}

func IsAttentionCycle(table *types.Table, elem []types.Entry) bool {
	return elem[table.IndexOfField(FieldAction)].Format("") == "Attend" && elem[table.IndexOfField(FieldDirect)].Format("") == ""
}

func IsGoalCycle(table *types.Table, elem []types.Entry) bool {
	return elem[table.IndexOfField(FieldAction)].Format("") == "Accomplish" && elem[table.IndexOfField(FieldDirect)].Format("") == "set goals"
}

func IsCompositeTask(table *types.Table, elem []types.Entry) bool {
	compositeIndirects := map[string]bool{
		"breakdown":      true,
		"habituality":    true,
		"incrementality": true,
		"regularity":     true,
	}
	return compositeIndirects[elem[table.IndexOfField(FieldIndirect)].Format("")]
}
