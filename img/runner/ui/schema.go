package ui

/*
Defines the constants of the time tracking schema for jql
*/

// TODO this is pretty common to any timedb tool so should be consolidated into a common library
const (
	// The status views share the names with the statuses themselves
	StateUnprocessed string = "Unprocessed"
	StatusActive     string = "Active"
	StatusSatisfied  string = "Satisfied"
	StatusSomeday    string = "Someday"

	ProjectsView string = "Projects"

	FieldAction          string = "Action"
	FieldBegin           string = "Begin"
	FieldDescription     string = "_Description"
	FieldEnd             string = "End"
	FieldFeed            string = "Feed"
	FieldIndirect        string = "Indirect"
	FieldLink            string = "Link"
	FieldLogDescription  string = "A_Description"
	FieldParent          string = "Parent"
	FieldPrimaryGoal     string = "Primary Goal"
	FieldSpan            string = "Param~Span"
	FieldStatus          string = "Status"
	FieldTask            string = "A Task"
	FieldNounDescription string = "Description"

	PrefixNouns     string = "nouns "
	TableAssertions string = "assertions"
	FieldArg1       string = "Arg1"
	FieldArg0       string = "Arg0"

	TableLog   string = "log"
	TableTasks string = "tasks"
	TableNouns string = "nouns"

	JQLName string = "jql"

	TopicView       string = "topic"
	TopicsView      string = "topics"
	TopicsQueryView string = "topicsQ"
	TypeView        string = "types"
	ResourceView    string = "resources"
	MetaView        string = "meta"
	RootTopic       string = "root"

	SpanDay   string = "Day"
	SpanWeek  string = "Week"
	SpanMonth string = "Month"
)

var Spans []string = []string{SpanDay, SpanWeek, SpanMonth}
