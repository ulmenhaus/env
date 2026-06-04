package timedb

import (
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

const (
	StateUnprocessed   = "Unprocessed"
	StatusAbandoned    = "Abandoned"
	StatusActive       = "Active"
	StatusExploring    = "Exploring"
	StatusFailed       = "Failed"
	StatusFresh        = "Fresh"
	StatusHabitual     = "Habitual"
	StatusIdea         = "Idea"
	StatusImplementing = "Implementing"
	StatusPending      = "Pending"
	StatusPlanned      = "Planned"
	StatusPlanning     = "Planning"
	StatusRevisit      = "Revisit"
	StatusSatisfied    = "Satisfied"
	StatusSomeday      = "Someday"

	FieldAction          = "Action"
	FieldArg0            = "Arg0"
	FieldArg1            = "Arg1"
	FieldBegin           = "Begin"
	FieldCode            = "Code"
	FieldContext         = "Context"
	FieldCoordinal       = "Coordinal"
	FieldDescription     = "_Description"
	FieldNounDescription = "Description"
	FieldDirect          = "Direct"
	FieldDisplayName     = "DisplayName"
	FieldDomain          = "Domain"
	FieldEnd             = "End"
	FieldFeed            = "Feed"
	FieldIdentifier      = "_Identifier"
	FieldIndirect        = "Indirect"
	FieldLink            = "Link"
	FieldLogDescription  = "A_Description"
	FieldModifier        = "A Modifier"
	FieldOrder           = "Order"
	FieldParent          = "Parent"
	FieldPrimaryGoal     = "Primary Goal"
	FieldProject         = "Project"
	FieldProjectName     = "A Name"
	FieldRelation        = "A Relation"
	FieldSkillset        = "Skillset"
	FieldSpan            = "Param~Span"
	FieldSrcLocation     = "SrcLocation"
	FieldStart           = "Param~Start"
	FieldStatus          = "Status"
	FieldTarget          = "A Target"
	FieldTask            = "A Task"
	FieldWorkdir         = "Workdir"

	TableActiveReminders = "vt.active_reminders"
	TableAssertions      = "assertions"
	TableContexts        = "contexts"
	TableKits            = "vt.kits"
	TableLog             = "log"
	TableNouns           = "nouns"
	TablePractices       = "vt.practices"
	TableReminders       = "vt.reminders"
	TableTasks           = "tasks"
	TableTools           = "vt.tools"
	JumpsTable           = "jumps"
	ProjectsTable        = "projects"
	BookmarksTable       = "bookmarks"
	ComponentsTable      = "components"

	JQLName         = "jql"
	ProjectsView    = "Projects"
	CountsView      = "counts"
	DomainView      = "Domains"
	FilterView      = "filters"
	LogView         = "log"
	MetaView        = "meta"
	NewPlanView     = "new_plan"
	NewPlansView    = "new_plans"
	NewTaskView     = "new_task"
	NextStateView   = "next_state"
	QueryTasksView  = "query_tasks"
	QueryView       = "query"
	ResourceView    = "resources"
	ResourcesView   = "Resources"
	RootTopic       = "root"
	Stage1View      = "Stage1"
	Stage2View      = "Stage2"
	Stage3View      = "Stage3"
	Stage4View      = "Stage4"
	StatsView       = "Status"
	SubDisplayView  = "SubDisplay"
	TasksView       = "tasks"
	TopicView       = "topic"
	TopicsQueryView = "topicsQ"
	TopicsView      = "topics"
	TypeView        = "types"
	TypesView       = "Types"
	WeeklyAttrsView = "weekly_attrs"

	FeedManual        = "manual"
	PrefixNouns       = "nouns "
	Today             = "Today"
	SpanDay           = "Day"
	SpanMonth         = "Month"
	SpanPending       = "Pending"
	SpanQuarter       = "Quarter"
	SpanWeek          = "Week"
	ValuePlanModifier = "Plan for"
)

var Spans = []string{Today, SpanDay, SpanWeek, SpanMonth, SpanPending}

var Span2Title = map[string]string{
	Today:       "Today",
	SpanDay:     "Active",
	SpanWeek:    "Later This Week",
	SpanMonth:   "Later This Month",
	SpanPending: "Pending",
}

func IsAttentionCycle(table *jqlpb.TableMeta, elem *jqlpb.Row) bool {
	return elem.Entries[api.IndexOfField(table.Columns, FieldAction)].Formatted == "Attend" &&
		elem.Entries[api.IndexOfField(table.Columns, FieldDirect)].Formatted == ""
}
