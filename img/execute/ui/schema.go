package ui

import (
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

/*
Defines the constants of the time tracking schema for jql
*/

const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusHabitual   string = "Habitual"
	StatusPending    string = "Pending"
	StatusPlanned    string = "Planned"
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
	FieldStart          string = "Param~Start"
	FieldStatus         string = "Status"
	FieldTask           string = "A Task"
	FieldArg0           string = "Arg0"
	FieldArg1           string = "Arg1"
	FieldARelation      string = "A Relation"
	FieldOrder          string = "Order"

	TableLog        string = "log"
	TableTasks      string = "tasks"
	TableAssertions string = "assertions"
	TableNouns      string = "nouns"
	TablePractices  string = "vt.practices"

	JQLName string = "jql"

	CountsView     string = "counts"
	TasksView      string = "tasks"
	LogView        string = "log"
	QueryTasksView string = "query_tasks"
	QueryView      string = "query"
	NewPlanView    string = "new_plan"
	NewPlansView   string = "new_plans"

	Today       string = "Today"
	SpanDay     string = "Day"
	SpanWeek    string = "Week"
	SpanMonth   string = "Month"
	SpanQuarter string = "Quarter"
	SpanPending string = "Pending"
)

var Spans []string = []string{"Today", SpanDay, SpanWeek, SpanMonth, SpanPending}

var Span2Title map[string]string = map[string]string{
	Today:       "Today",
	SpanDay:     "Active",
	SpanWeek:    "Later This Week",
	SpanMonth:   "Later This Month",
	SpanPending: "Pending",
}

func IsAttentionCycle(table *jqlpb.TableMeta, elem *jqlpb.Row) bool {
	return elem.Entries[api.IndexOfField(table.Columns, FieldAction)].Formatted == "Attend" && elem.Entries[api.IndexOfField(table.Columns, FieldDirect)].Formatted == ""
}
