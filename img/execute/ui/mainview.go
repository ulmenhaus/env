package ui

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/lib/go/timedb"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

// MainViewMode is the current mode of the MainView.
// It captures the state of the MainView object with
// respect to actions and state transitions between them.
type MainViewMode int

const (
	MainViewModeListBar MainViewMode = iota
	MainViewModeQueryingForTask
	MainViewModeQueryingForNewPlan
	MainViewModeQueryingForPlansSubset
	MainViewModeQueryingForNounNextState
)

const (
	blackTextEscape = "\033[30m"
	whiteBackEscape = "\033[47m"
	boldTextEscape  = "\033[1m"
	resetEscape     = "\033[0m"
)

// TaskViewMode is the way in which tasks are presented
type TaskViewMode int

const (
	TaskViewModeListBar TaskViewMode = iota
	TaskViewModeListCycles
)

var (
	ctx = context.Background()
)

// A MainView is the overall view including a project list
// and a detailed view of the current project
type MainView struct {
	dbms   api.JQL_DBMS
	tables map[string]*jqlpb.TableMeta

	MainViewMode MainViewMode
	TaskViewMode TaskViewMode

	// maps span to tasks of that span
	tasks map[string]([]*jqlpb.Row)
	span  string
	log   []*jqlpb.Row

	// today
	cachedTodayTasks []string
	today            []DayItem
	today2item       map[string]DayItemMeta // keyed by reminderArg0
	ix2item          map[int]DayItem
	reminderCache    map[string]*reminderInfo // reminderArg0 → info

	// state used for searching tasks
	topicQ          string
	unfilteredTasks []string
	filteredTasks   []string
	queryCallback   func(taskPK string) error

	// state used for querying for a new plan / reminder
	newPlanTaskPK              string
	newPlanDescription         string
	newReminderInsertAfterPK   string // .Entry assn PK of the item under cursor when 'i' was pressed

	// state used for querying for a subset of plans
	planSelections   []PlanSelectionItem
	substitutingPlan bool

	// state used for focus mode
	focusing             bool
	justSwitchedGrouping bool

	// state used for prompting for next noun state
	nounSwitchingStatePK string
	nounStateNextState   string

	// initialization params for reentrance
	preselectTask       string
	injectMatchingTasks bool

	// bottom display data
	weeklyIntention  string
	weeklyTouchstone string
}

type DayItem struct {
	Break        string
	Description  string
	PK           string
	ReminderArg0 string // non-empty for reminder FK entries
}

type DayItemMeta struct {
	TaskPK      string
	AssertionPK string
}

// reminderInfo holds cached data for a reminder entity in the current day plan.
type reminderInfo struct {
	taskPK       string
	taskArg0     string
	checkText    string // empty for task-level reminders
	description  string // checkText if set, else taskPK
	status       string // raw status: Awaiting, Ready, Done, Elided, Failed
	statusAssnPK string
}

type PlanSelectionItem struct {
	Plan   string
	Marked bool
}

// reminderToPlace collects the data needed to create or place a reminder.
// dayPlanGroup and dayPlanOrder are populated from habit metadata and used by
// computeEntrySequence to interleave new entries into the day plan.
type reminderToPlace struct {
	taskPK       string
	checkText    string
	dayPlanGroup string
	dayPlanOrder int
}

// dayPlanEntry is a snapshot of an existing .Entry assertion on the day plan.
type dayPlanEntry struct {
	pk    string
	arg1  string
	order int
}

// habitPlacementMeta carries DayPlanGroup and DayPlanOrder resolved from a habit task.
type habitPlacementMeta struct {
	dayPlanGroup string
	dayPlanOrder int
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, dbms api.JQL_DBMS, preselectTask string, injectMatchingTasks bool) (*MainView, error) {
	rand.Seed(time.Now().UnixNano())
	mv := &MainView{
		dbms:                dbms,
		preselectTask:       preselectTask,
		injectMatchingTasks: injectMatchingTasks,
	}
	return mv, mv.load(g)
}

func (mv *MainView) load(g *gocui.Gui) error {
	mv.MainViewMode = MainViewModeListBar
	mv.tasks = map[string]([]*jqlpb.Row){}
	mv.span = timedb.Today
	tables, err := api.GetTables(ctx, mv.dbms)
	if err != nil {
		return err
	}
	mv.tables = tables
	return mv.refreshView(g)
}

func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if mv.MainViewMode == MainViewModeQueryingForTask {
		mv.editSearch(v, key, ch, mod)
		return
	} else if mv.MainViewMode == MainViewModeQueryingForNewPlan {
		mv.editNewPlan(v, key, ch, mod)
		return
	}
}

func (mv *MainView) editSearch(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.topicQ) != 0 {
			mv.topicQ = mv.topicQ[:len(mv.topicQ)-1]
		}
	} else if key == gocui.KeySpace {
		mv.topicQ += " "
	} else {
		mv.topicQ += string(ch)
	}
	mv.setTopics()
}

func (mv *MainView) editNewPlan(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.newPlanDescription) != 0 {
			mv.newPlanDescription = mv.newPlanDescription[:len(mv.newPlanDescription)-1]
		}
	} else if key == gocui.KeySpace {
		mv.newPlanDescription += " "
	} else {
		mv.newPlanDescription += string(ch)
	}
}

func (mv *MainView) selectQueryItem(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	selected := mv.filteredTasks[ix]
	err := g.DeleteView(timedb.QueryTasksView)
	if err != nil {
		return err
	}
	err = g.DeleteView(timedb.QueryView)
	if err != nil {
		return err
	}
	mv.topicQ = ""
	mv.MainViewMode = MainViewModeListBar
	return mv.queryCallback(selected)
}

func (mv *MainView) setTopics() error {
	mv.filteredTasks = []string{}
	for _, task := range mv.unfilteredTasks {
		if strings.Contains(strings.ToLower(task), mv.topicQ) {
			mv.filteredTasks = append(mv.filteredTasks, task)
		}
	}
	return nil
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	if mv.MainViewMode == MainViewModeQueryingForTask {
		return mv.queryForTaskLayout(g)
	} else if mv.MainViewMode == MainViewModeQueryingForNewPlan {
		return mv.queryForNewPlanLayout(g)
	} else if mv.MainViewMode == MainViewModeQueryingForPlansSubset {
		return mv.queryForPlanSubsetLayout(g)
	} else if mv.MainViewMode == MainViewModeQueryingForNounNextState {
		return mv.queryForNounNextStateLayout(g)
	} else {
		return mv.listTasksLayout(g)
	}
}

func (mv *MainView) createNewPlanFromInput(g *gocui.Gui, v *gocui.View) error {
	err := g.DeleteView(timedb.NewPlanView)
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	err = mv.createNewReminder(g, mv.newPlanTaskPK, mv.newPlanDescription)
	if err != nil {
		return err
	}
	mv.newPlanTaskPK = ""
	mv.newPlanDescription = ""
	return nil
}

func (mv *MainView) createNewReminder(g *gocui.Gui, taskPK, checkText string) error {
	dayPlan, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if dayPlan == nil {
		return nil
	}
	tasksTable := mv.tables[timedb.TableTasks]
	dayPlanPK := dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted

	// Write .Check assertion on the task
	checkPK := fmt.Sprintf("%d", rand.Int63())
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      timedb.TableAssertions,
		Pk:         checkPK,
		InsertOnly: true,
		Fields: map[string]string{
			timedb.FieldRelation: ".Check",
			timedb.FieldArg0:      fmt.Sprintf("tasks %s", taskPK),
			timedb.FieldArg1:      checkText,
		},
	})
	if err != nil {
		return err
	}

	// Determine insertion order: after the cursor item or at the end
	insertOrder, err := mv.resolveInsertOrder(dayPlanPK)
	if err != nil {
		return err
	}
	mv.newReminderInsertAfterPK = ""

	todayStr := time.Now().Format("2006-01-02")
	if err = mv.createReminderEntity(dayPlanPK, taskPK, checkText, todayStr, insertOrder); err != nil {
		return err
	}
	err = mv.save()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

// resolveInsertOrder returns the Order to use for a new .Entry assertion.
// If newReminderInsertAfterPK is set, it shifts subsequent entries and returns
// insertAfterOrder+1. Otherwise it returns maxOrder+1.
func (mv *MainView) resolveInsertOrder(dayPlanPK string) (int, error) {
	// Query all .Entry assertions sorted by order
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", dayPlanPK)}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Entry"}}},
			},
		}},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return 0, err
	}
	assnTable := mv.tables[timedb.TableAssertions]
	if len(resp.Rows) == 0 {
		return 0, nil
	}

	if mv.newReminderInsertAfterPK == "" {
		// Append to end
		lastOrder, _ := strconv.Atoi(resp.Rows[len(resp.Rows)-1].Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
		return lastOrder + 1, nil
	}

	// Find the insertion point and shift subsequent entries
	insertAfterOrder := -1
	for _, row := range resp.Rows {
		pk := row.Entries[api.GetPrimary(assnTable.Columns)].Formatted
		if pk == mv.newReminderInsertAfterPK {
			insertAfterOrder, _ = strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
			break
		}
	}
	if insertAfterOrder == -1 {
		// Fallback: append to end
		lastOrder, _ := strconv.Atoi(resp.Rows[len(resp.Rows)-1].Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
		return lastOrder + 1, nil
	}

	// Shift entries with order > insertAfterOrder upward (in reverse to avoid collisions)
	for i := len(resp.Rows) - 1; i >= 0; i-- {
		row := resp.Rows[i]
		ord, _ := strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
		if ord > insertAfterOrder {
			pk := row.Entries[api.GetPrimary(assnTable.Columns)].Formatted
			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				UpdateOnly: true,
				Table:      timedb.TableAssertions,
				Pk:         pk,
				Fields:     map[string]string{timedb.FieldOrder: fmt.Sprintf("%d", ord+1)},
			})
			if err != nil {
				return 0, err
			}
		}
	}
	return insertAfterOrder + 1, nil
}

// TODO: createNewPlan can be deleted once substitutePlanSelectionsForTask migrates to the new assertion-based reminder model.
func (mv *MainView) createNewPlan(g *gocui.Gui, taskPK, description string) error {
	assnTable := mv.tables[timedb.TableAssertions]
	newOrder := 0
	plansResp, err := mv.queryPlans([]string{taskPK})
	if err != nil {
		return err
	}
	for _, plan := range plansResp.Rows {
		orderInt, err := strconv.Atoi(plan.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
		if err != nil {
			continue
		}
		if orderInt >= newOrder {
			newOrder = orderInt + 1
		}
	}

	// pk doesn't really matter here so using a random integer
	pk := fmt.Sprintf("%d", rand.Int63())
	fields := map[string]string{
		timedb.FieldArg0:      fmt.Sprintf("tasks %s", taskPK),
		timedb.FieldArg1:      fmt.Sprintf("[ ] %s", description),
		timedb.FieldRelation: ".Plan",
		timedb.FieldOrder:     fmt.Sprintf("%d", newOrder),
	}
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  timedb.TableAssertions,
		Pk:     pk,
		Fields: fields,
	})
	if err != nil {
		return err
	}
	err = mv.insertDayPlan(g, description, 0)
	if err != nil {
		return err
	}
	err = mv.save()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

// TODO: insertDayPlan can be deleted once wrapTaskInRamps and substitute flows migrate to the new assertion-based reminder model.
func (mv *MainView) insertDayPlan(g *gocui.Gui, description string, delta int) error {
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	tasksView, err := g.View(timedb.TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	insertsAfter := mv.ix2item[ix]
	dayPlan, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	dayPlanLink := fmt.Sprintf("tasks %s", dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)
	existingTodos, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: dayPlanLink}},
					},
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Do timedb.Today"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return err
	}
	dayOrder := 0
	for _, row := range existingTodos.Rows {
		if row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted == insertsAfter.Description {
			dayOrder, err = strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
			if err != nil {
				return err
			}
		}
	}
	dayOrder += delta
	updated := []string{}
	for _, row := range existingTodos.Rows {
		orderInt, err := strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted)
		if err != nil {
			return err
		}
		if orderInt > dayOrder {
			pk := row.Entries[api.GetPrimary(assnTable.Columns)].Formatted
			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				UpdateOnly: true,
				Table:      timedb.TableAssertions,
				Pk:         pk,
				Fields:     map[string]string{timedb.FieldOrder: fmt.Sprintf("%d", orderInt+1)},
			})
			if err != nil {
				return err
			}
			// NOTE We sync the row pks in reverse order so that we avoid a row overwriting its successor
			updated = append([]string{pk}, updated...)
		}
	}
	err = mv.syncPKs(timedb.TableAssertions, updated)
	if err != nil {
		return err
	}

	fields := map[string]string{
		timedb.FieldArg0:      dayPlanLink,
		timedb.FieldArg1:      fmt.Sprintf("[ ] %s", description),
		timedb.FieldRelation: ".Do timedb.Today",
		timedb.FieldOrder:     fmt.Sprintf("%d", dayOrder+1),
	}
	pk := fmt.Sprintf("%d", rand.Int63())
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      timedb.TableAssertions,
		Pk:         pk,
		Fields:     fields,
		InsertOnly: true,
	})
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) syncPKs(table string, updated []string) error {
	// TODO it's inefficient to run this macro for each key separately when we could
	// have a macro interface that supports multiple selected keys
	//
	// When we implement this, the interface must preserve row order to prevent pks overwriting
	// each other
	for _, pk := range updated {
		view := api.MacroCurrentView{
			Table:            table,
			PrimarySelection: pk,
		}
		_, err := api.RunMacro(ctx, mv.dbms, "jql-timedb-setpk --v2", view, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) queryForNewPlanLayout(g *gocui.Gui) error {
	maxX, _ := g.Size()
	newPlanView, err := g.SetView(timedb.NewPlanView, 4, 5, maxX-4, 9)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	newPlanView.Editable = true
	newPlanView.Editor = mv
	g.SetCurrentView(timedb.NewPlanView)
	newPlanView.Clear()
	newPlanView.Write([]byte("New Plan Description\n"))
	newPlanView.Write([]byte(mv.newPlanDescription))
	return nil
}

func (mv *MainView) queryForTaskLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	queryTasksView, err := g.SetView(timedb.QueryTasksView, 4, 5, maxX-4, maxY-7)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	queryTasksView.Highlight = true
	queryTasksView.SelBgColor = gocui.ColorWhite
	queryTasksView.SelFgColor = gocui.ColorBlack
	queryTasksView.Editable = true
	queryTasksView.Editor = mv
	queryTasksView.Clear()
	g.SetCurrentView(timedb.QueryTasksView)
	for _, task := range mv.filteredTasks {
		spaces := maxX - len(task)
		if spaces > 0 {
			task += strings.Repeat(" ", spaces)
		}
		queryTasksView.Write([]byte(task + "\n"))
	}
	query, err := g.SetView(timedb.QueryView, 4, maxY-7, maxX-4, maxY-5)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	query.Clear()
	query.Write([]byte(mv.topicQ))
	return nil
}

func (mv *MainView) listTasksLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	counts, err := g.SetView(timedb.CountsView, 0, 0, (maxX*3)/4, 2)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	counts.Clear()
	for _, span := range timedb.Spans {
		prefix := "  "
		if span == mv.span {
			prefix = blackTextEscape + whiteBackEscape + prefix
		}
		suffix := fmt.Sprintf("%s(%d)  %s", boldTextEscape, len(mv.tasks[span]), resetEscape)
		if len(mv.tasks[span]) == 0 {
			suffix = "  "
		}
		if span == mv.span {
			suffix += resetEscape
		}
		fmt.Fprintf(counts, "%s%s %s    ", prefix, timedb.Span2Title[span], suffix)
	}
	tasks, err := g.SetView(timedb.TasksView, 0, 3, (maxX*3)/4, maxY-4)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	tasks.Clear()
	weekly, err := g.SetView(timedb.WeeklyAttrsView, 0, maxY-4, (maxX*3)/4, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	weekly.Clear()
	if mv.weeklyIntention != "" {
		weekly.Write([]byte(fmt.Sprintf("Intention:  %s\n", mv.weeklyIntention)))
	}
	if mv.weeklyTouchstone != "" {
		weekly.Write([]byte(fmt.Sprintf("Touchstone: %s\n", mv.weeklyTouchstone)))
	}
	g.SetCurrentView(timedb.TasksView)
	log, err := g.SetView(timedb.LogView, (maxX*3/4)+1, 0, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	log.Clear()
	tasks.SelBgColor = gocui.ColorWhite
	tasks.SelFgColor = gocui.ColorBlack
	tasks.Highlight = true

	tabulated, err := mv.tabulatedTasks(g, tasks)
	if err != nil {
		return err
	}
	for _, desc := range tabulated {
		fmt.Fprintf(tasks, "%s\n", desc)
	}

	logTable := mv.tables[timedb.TableLog]
	logDescriptionField := api.IndexOfField(logTable.Columns, timedb.FieldLogDescription)
	beginField := api.IndexOfField(logTable.Columns, timedb.FieldBegin)
	endField := api.IndexOfField(logTable.Columns, timedb.FieldEnd)

	for _, logEntry := range mv.log {
		fmt.Fprintf(
			log, "%s\n    %s - %s\n\n",
			logEntry.Entries[logDescriptionField].Formatted,
			logEntry.Entries[beginField].Formatted,
			logEntry.Entries[endField].Formatted,
		)
	}

	return nil
}

func (mv *MainView) tabulatedTasks(g *gocui.Gui, v *gocui.View) ([]string, error) {
	if mv.span == timedb.Today {
		wasNil := mv.cachedTodayTasks == nil
		today, err := mv.todayTasks()
		if err != nil {
			return nil, err
		}
		mv.cachedTodayTasks = today
		if mv.preselectTask != "" {
			for i, item := range mv.ix2item {
				if info, ok := mv.reminderCache[item.ReminderArg0]; ok && info.taskPK == mv.preselectTask {
					err = v.SetCursor(0, i)
					if err != nil {
						return nil, err
					}
				}
			}
			if mv.injectMatchingTasks {
				_, err = mv.InjectTaskWithAllMatching(g, v, false)
				if err != nil {
					return nil, err
				}
			}
			mv.preselectTask = ""
		} else if wasNil || mv.justSwitchedGrouping {
			mv.justSwitchedGrouping = false
			mv.selectNextFreeTask(g, v)
		}
		return mv.cachedTodayTasks, nil
	}
	tasksTable := mv.tables[timedb.TableTasks]
	projectField := api.IndexOfField(tasksTable.Columns, timedb.FieldPrimaryGoal)
	descriptionField := api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)

	// 10 char buffer
	buffer := 10
	maxChars := buffer
	for _, task := range mv.tasks[mv.span] {
		taskChars := len(task.Entries[descriptionField].Formatted) + buffer
		if taskChars > maxChars {
			maxChars = taskChars
		}
	}

	toret := []string{}

	for _, task := range mv.tasks[mv.span] {
		taskBuffer := maxChars - len(task.Entries[descriptionField].Formatted)
		toret = append(toret,
			fmt.Sprintf(" %s%s%s",
				task.Entries[descriptionField].Formatted,
				strings.Repeat(" ", taskBuffer),
				task.Entries[projectField].Formatted,
			))
	}
	return toret, nil
}

func (mv *MainView) todayBreakdown() ([]DayItem, error) {
	if mv.TaskViewMode != TaskViewModeListCycles {
		return mv.today, nil
	}
	tasksTable := mv.tables[timedb.TableTasks]
	today := []DayItem{}
	for _, item := range mv.today {
		// Fall back to using the item's description as its attention
		// cycle if this is a one-off or we can't find its primary for some
		// reason
		brk := item.Description
		meta := mv.today2item[item.ReminderArg0]
		taskPK := meta.TaskPK
		if taskPK == "" {
			if info, ok := mv.reminderCache[item.ReminderArg0]; ok {
				taskPK = info.taskPK
			}
		}
		resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
			Table: timedb.TableTasks,
			Pk:    taskPK,
		})
		if err == nil {
			task, err := mv.retrieveAttentionCycle(tasksTable, resp.Row)
			if err == nil {
				brk = task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
			}
		}
		today = append(today, DayItem{
			Break:       brk,
			Description: item.Description,
			PK:          item.PK,
		})

	}
	return today, nil
}

func (mv *MainView) todayTasks() ([]string, error) {
	tasks := []string{}
	ix2item := map[int]DayItem{}
	type brk struct {
		description string
		items       []DayItem
		done        int
	}
	brks := []*brk{}
	currentBreak := &brk{}
	breakdown, err := mv.todayBreakdown()
	if err != nil {
		return nil, err
	}
	for _, item := range breakdown {
		if item.Break != currentBreak.description {
			currentBreak = &brk{
				description: item.Break,
			}
			brks = append(brks, currentBreak)
		}
		currentBreak.items = append(currentBreak.items, item)
		if isDayTaskDone(item.Description) {
			currentBreak.done += 1
		}
	}
	passedFirstWithUnfinished := false
	for _, brk := range brks {
		if mv.focusing && (passedFirstWithUnfinished || brk.done == len(brk.items)) {
			tasks = append(tasks, fmt.Sprintf("[%d/%d] %s", brk.done, len(brk.items), brk.description))
		} else {
			tasks = append(tasks, brk.description)
			for _, item := range brk.items {
				ix2item[len(tasks)] = item
				tasks = append(tasks, " "+item.Description)
			}
			passedFirstWithUnfinished = brk.done != len(brk.items)
		}
	}
	mv.ix2item = ix2item
	return tasks, nil
}

func (mv *MainView) saveContents(g *gocui.Gui, v *gocui.View) error {
	return mv.save()
}

func (mv *MainView) save() error {
	_, err := mv.dbms.Persist(ctx, &jqlpb.PersistRequest{})
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	err := g.SetKeybinding(timedb.TasksView, 'k', gocui.ModNone, mv.cursorUp)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'j', gocui.ModNone, mv.cursorDown)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.TasksView, gocui.KeyEnter, gocui.ModNone, mv.logTime); err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'e', gocui.ModNone, mv.runProcedure)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'w', gocui.ModNone, mv.openLink)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'i', gocui.ModNone, mv.bumpStatus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'I', gocui.ModNone, mv.degradeStatus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'l', gocui.ModNone, mv.nextSpan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'h', gocui.ModNone, mv.prevSpan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'a', gocui.ModNone, mv.switchModes)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'f', gocui.ModNone, mv.toggleFocus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'X', gocui.ModNone, mv.refreshTasks)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'P', gocui.ModNone, mv.wrapTaskInRamps)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.NewPlansView, 'x', gocui.ModNone, mv.toggleAllPlans)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'x', gocui.ModNone, mv.taskMarker(timedb.StatusSatisfied))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'z', gocui.ModNone, mv.taskMarker(timedb.StatusFailed))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'Z', gocui.ModNone, mv.taskMarker(timedb.StatusAbandoned))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 'd', gocui.ModNone, mv.deleteDayPlan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(timedb.TasksView, 's', gocui.ModNone, mv.substituteTaskWithPrompt)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.QueryTasksView, gocui.KeyEnter, gocui.ModNone, mv.selectQueryItem); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NewPlanView, gocui.KeyEnter, gocui.ModNone, mv.createNewPlanFromInput); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NewPlansView, 'j', gocui.ModNone, mv.basicCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NewPlansView, 'k', gocui.ModNone, mv.basicCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NewPlansView, gocui.KeySpace, gocui.ModNone, mv.markPlanSelection); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NewPlansView, gocui.KeyEnter, gocui.ModNone, mv.substitutePlanSelections); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NextStateView, 'j', gocui.ModNone, mv.basicCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NextStateView, 'k', gocui.ModNone, mv.basicCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(timedb.NextStateView, gocui.KeyEnter, gocui.ModNone, mv.selectNextNounState); err != nil {
		return err
	}
	return nil
}

func (mv *MainView) selectNextFreeTask(g *gocui.Gui, v *gocui.View) {
	for i, task := range mv.cachedTodayTasks {
		// TODO(rabrams) bit of a hack here to identify which tasks are undone
		if strings.HasPrefix(task, " [ ]") {
			v.SetCursor(0, i)
			return
		}
	}
}

func (mv *MainView) nextSpan(g *gocui.Gui, v *gocui.View) error {
	ixs := map[string]int{}
	for ix, span := range timedb.Spans {
		ixs[span] = ix
	}
	mv.span = timedb.Spans[(ixs[mv.span]+1)%len(timedb.Spans)]
	if mv.span == timedb.Today {
		mv.selectNextFreeTask(g, v)
	} else {
		v.SetCursor(0, 0)
	}
	return mv.refreshView(g)
}

func (mv *MainView) prevSpan(g *gocui.Gui, v *gocui.View) error {
	ixs := map[string]int{}
	for ix, span := range timedb.Spans {
		ixs[span] = ix
	}
	prevIx := (ixs[mv.span] - 1)
	if prevIx == -1 {
		prevIx = len(timedb.Spans) - 1
	}
	mv.span = timedb.Spans[prevIx]
	if mv.span == timedb.Today {
		mv.selectNextFreeTask(g, v)
	} else {
		v.SetCursor(0, 0)
	}
	return mv.refreshView(g)
}

func (mv *MainView) queryForTask(g *gocui.Gui, v *gocui.View, callback func(cycle string) error) error {
	mv.MainViewMode = MainViewModeQueryingForTask
	mv.queryCallback = callback
	return nil
}

func (mv *MainView) setTaskList(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	currentPK := ""
	item, ok := mv.ix2item[ix]
	if ok && item.ReminderArg0 != "" {
		if info, ok := mv.reminderCache[item.ReminderArg0]; ok {
			currentPK = info.taskPK
		}
	}
	inProgress, err := mv.queryInProgressTasks(currentPK)
	if err != nil {
		return err
	}
	if currentPK != "" {
		inProgress = append([]string{currentPK}, inProgress...)
	}
	mv.unfilteredTasks = inProgress
	mv.filteredTasks = mv.unfilteredTasks
	return nil
}

func (mv *MainView) insertNewPlan(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	if item, ok := mv.ix2item[ix]; ok {
		mv.newReminderInsertAfterPK = item.PK
	} else {
		mv.newReminderInsertAfterPK = ""
	}
	err := mv.setTaskList(g, v)
	if err != nil {
		return err
	}
	return mv.queryForTask(g, v, func(taskPK string) error {
		return mv.queryForNewPlan(taskPK)
	})
}

func (mv *MainView) SelectTask(g *gocui.Gui, v *gocui.View, ret func(taskPK string) error) error {
	err := mv.setTaskList(g, v)
	if err != nil {
		return err
	}
	return mv.queryForTask(g, v, ret)
}

func (mv *MainView) queryForNewPlan(taskPK string) error {
	mv.MainViewMode = MainViewModeQueryingForNewPlan
	mv.newPlanTaskPK = taskPK
	return nil
}

func (mv *MainView) bumpStatus(g *gocui.Gui, v *gocui.View) error {
	if mv.span == timedb.Today {
		return mv.insertNewPlan(g, v)
	}
	return mv.addToStatus(g, v, 1)
}

func (mv *MainView) degradeStatus(g *gocui.Gui, v *gocui.View) error {
	return mv.addToStatus(g, v, -1)
}

func (mv *MainView) addToStatus(g *gocui.Gui, v *gocui.View, delta int) error {
	// TODO getting selected task is very common. Should factor out.
	tasksTable := mv.tables[timedb.TableTasks]
	var cy, oy int
	view, err := g.View(timedb.TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[mv.span][oy+cy]
	pk := selectedTask.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)].Formatted

	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  timedb.TableTasks,
		Pk:     pk,
		Column: timedb.FieldStatus,
		Amount: int32(delta),
	})
	if err != nil {
		return err
	}
	err = mv.saveContents(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) runProcedure(g *gocui.Gui, v *gocui.View) error {
	pk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return err
	}
	view := api.MacroCurrentView{
		Table:            timedb.TableTasks,
		PrimarySelection: pk,
	}
	_, err = api.RunMacro(ctx, mv.dbms, "jql-timedb-run-procedure", view, true)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) openLink(g *gocui.Gui, v *gocui.View) error {
	pk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return err
	}
	tasksTable := mv.tables[timedb.TableTasks]
	nounsTable := mv.tables[timedb.TableNouns]
	task, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableTasks,
		Pk:    pk,
	})
	if err != nil {
		return err
	}
	direct := task.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDirect)].Formatted
	obj, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableNouns,
		Pk:    direct,
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("txtopen", obj.Row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldLink)].Formatted)
	return cmd.Run()
}

func (mv *MainView) logTime(g *gocui.Gui, v *gocui.View) error {
	tasksTable := mv.tables[timedb.TableTasks]
	logTable := mv.tables[timedb.TableLog]
	var cy, oy int
	view, err := g.View(timedb.TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[mv.span][oy+cy]

	// XXX this is a really janky way to check the value of the time entry
	// and create the next valid entry
	if len(mv.log) == 0 {
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)].Formatted), selectedTask, false)
		if err != nil {
			return err
		}
	} else if mv.log[0].Entries[api.IndexOfField(logTable.Columns, timedb.FieldEnd)].Formatted == "31 Dec 1969 16:00:00" {
		pk := mv.log[0].Entries[api.IndexOfField(logTable.Columns, timedb.FieldLogDescription)].Formatted
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      timedb.TableLog,
			Pk:         pk,
			Fields:     map[string]string{timedb.FieldEnd: ""},
		})
		if err != nil {
			return err
		}
	} else {
		pk := mv.log[0].Entries[api.IndexOfField(logTable.Columns, timedb.FieldLogDescription)].Formatted
		ordinal := pk[len(pk)-5 : len(pk)-1]
		ordinalI, err := strconv.Atoi(ordinal)
		if err != nil {
			return err
		}
		newPK := fmt.Sprintf("%s%04d)", pk[:len(pk)-5], ordinalI+1)
		err = mv.newTime(g, newPK, selectedTask, false)
		if err != nil {
			return err
		}
	}
	err = mv.saveContents(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) newTime(g *gocui.Gui, pk string, selectedTask *jqlpb.Row, andFinish bool) error {
	tasksTable := mv.tables[timedb.TableTasks]
	fields := map[string]string{
		timedb.FieldBegin: "",
		timedb.FieldTask:  selectedTask.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)].Formatted,
	}
	if andFinish {
		fields[timedb.FieldEnd] = ""
	}
	_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  timedb.TableLog,
		Pk:     pk,
		Fields: fields,
	})
	return err
}

func (mv *MainView) basicCursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	if err := v.SetCursor(cx, cy+1); err != nil {
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) basicCursorUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	if err := v.SetCursor(cx, cy-1); err != nil {
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	_, oy := v.Origin()
	delta := 1
	if mv.span == timedb.Today {
		for {
			ix := cy + oy + delta
			// TODO(rabrams) would be good to comprehensively stop the cursor at the end of each
			// span's list
			if ix >= len(mv.cachedTodayTasks) {
				break
			}
			// TODO(rabrams) bit of a hack here to identify which tasks can be skipped
			// because they're already done -- NOTE we don't do the same for cursor-up
			// so you can backtrack to those if you want
			if strings.HasPrefix(mv.cachedTodayTasks[ix], " [ ]") {
				break
			}
			delta += 1
		}
	}
	if err := v.SetCursor(cx, cy+delta); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+delta); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	ox, oy := v.Origin()
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) GetTodayPlanPK() (string, error) {
	today, err := mv.queryDayPlan()
	if err != nil {
		return "", err
	}
	if today == nil {
		return "", nil
	}
	tasksTable := mv.tables[timedb.TableTasks]
	return today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted, nil
}

func (mv *MainView) ResolveSelectedPK(g *gocui.Gui) (string, error) {
	var cy, oy int
	view, err := g.View(timedb.TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return "", err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}
	ix := oy + cy
	if mv.span == timedb.Today {
		item, ok := mv.ix2item[ix]
		if !ok {
			return "", fmt.Errorf("index beyond bounds: %d", ix)
		}
		if item.ReminderArg0 != "" {
			if info, ok := mv.reminderCache[item.ReminderArg0]; ok && info.taskPK != "" {
				return info.taskPK, nil
			}
		}
		return stripDayPlanPrefix(item.Description), nil
	} else {
		tasksTable := mv.tables[timedb.TableTasks]
		selectedTask := mv.tasks[mv.span][ix]
		return selectedTask.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)].Formatted, nil
	}
}

func (mv *MainView) GetSelectedReminderArg0(g *gocui.Gui) (string, error) {
	var cy, oy int
	view, err := g.View(timedb.TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return "", err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}
	ix := oy + cy
	item, ok := mv.ix2item[ix]
	if !ok {
		return "", nil
	}
	return item.ReminderArg0, nil
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	err := mv.refreshWeeklyDisplays()
	if err != nil {
		return err
	}
	tasksTable := mv.tables[timedb.TableTasks]
	descriptionField := api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)
	projectField := api.IndexOfField(tasksTable.Columns, timedb.FieldPrimaryGoal)
	spanField := api.IndexOfField(tasksTable.Columns, timedb.FieldSpan)
	statusField := api.IndexOfField(tasksTable.Columns, timedb.FieldStatus)

	active, err := mv.queryAllTasks(timedb.StatusPlanned, timedb.StatusActive)
	if err != nil {
		return err
	}
	mv.tasks = map[string]([]*jqlpb.Row){}
	for _, task := range active.Rows {
		span := task.Entries[spanField].Formatted
		// qurater scope tasks are good to keep an eye on, but to keep the
		// UX simple let's lump then in with the tasks for "this month"
		if span == timedb.SpanQuarter {
			span = timedb.SpanMonth
		}
		// If the task has already been started then mark it as active for today
		if task.Entries[statusField].Formatted == "Active" {
			span = timedb.SpanDay
		}
		if mv.TaskViewMode == TaskViewModeListCycles {
			task, err = mv.retrieveAttentionCycle(tasksTable, task)
			if err != nil {
				return err
			}
		}
		mv.tasks[span] = append(mv.tasks[span], task)
	}

	pending, err := mv.queryPendingNoImplements()
	if err != nil {
		return err
	}
	for _, task := range pending {
		if mv.TaskViewMode == TaskViewModeListCycles {
			task, err = mv.retrieveAttentionCycle(tasksTable, task)
			if err != nil {
				return err
			}
		}
		mv.tasks[timedb.SpanPending] = append(mv.tasks[timedb.SpanPending], task)
	}
	for span := range mv.tasks {
		sort.Slice(mv.tasks[span], func(i, j int) bool {
			iRes := mv.tasks[span][i].Entries[projectField].Formatted
			jRes := mv.tasks[span][j].Entries[projectField].Formatted

			iDesc := mv.tasks[span][i].Entries[descriptionField].Formatted
			jDesc := mv.tasks[span][j].Entries[descriptionField].Formatted

			return (iRes < jRes) || ((iRes == jRes) && iDesc < jDesc)
		})
	}

	var cy, oy int
	view, err := g.View(timedb.TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	mv.log = []*jqlpb.Row{}
	if mv.span != timedb.Today {
		if oy+cy < len(mv.tasks[mv.span]) {
			selectedTask := mv.tasks[mv.span][oy+cy]
			resp, err := mv.queryLogs(selectedTask)
			if err != nil {
				return err
			}
			mv.log = resp.Rows
		}
	}
	return mv.refreshToday()
}

func (mv *MainView) refreshWeeklyDisplays() error {
	intentions, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table:   timedb.TableTasks,
		OrderBy: timedb.FieldStart,
		Dec:     true,
		Limit:   1,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Intend"}},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if len(intentions.Rows) == 0 {
		mv.weeklyIntention = ""
	} else {
		mv.weeklyIntention = intentions.Rows[0].Entries[api.IndexOfField(intentions.Columns, timedb.FieldDirect)].Formatted
	}
	touchstones, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table:   timedb.TableTasks,
		OrderBy: timedb.FieldStart,
		Dec:     true,
		Limit:   1,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Ritualize"}},
					},
				},
			},
		},
	})
	if len(touchstones.Rows) == 0 {
		mv.weeklyTouchstone = ""
	} else {
		mv.weeklyTouchstone = touchstones.Rows[0].Entries[api.IndexOfField(touchstones.Columns, timedb.FieldDirect)].Formatted
	}
	return nil
}

func (mv *MainView) refreshToday() error {
	mv.today = []DayItem{}
	mv.reminderCache = map[string]*reminderInfo{}
	mv.today2item = map[string]DayItemMeta{}

	today, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if today == nil {
		return nil
	}
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	dayPlanArg0 := fmt.Sprintf("tasks %s", today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)

	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: dayPlanArg0}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Entry"}}},
			},
		}},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return err
	}

	const reminderFKPrefix = "@{vt.reminders "
	var reminderArg0s []string
	reminderArg02EntryPK := map[string]string{}

	// First pass: collect reminder arg0s and entry assertion PKs
	for _, row := range resp.Rows {
		arg1 := row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		entryPK := row.Entries[api.GetPrimary(assnTable.Columns)].Formatted
		if strings.HasPrefix(arg1, reminderFKPrefix) && strings.HasSuffix(arg1, "}") {
			arg0 := arg1[len(reminderFKPrefix) : len(arg1)-1]
			reminderArg0s = append(reminderArg0s, arg0)
			reminderArg02EntryPK[arg0] = entryPK
		}
	}

	// Batch-query all attributes for these reminders
	if len(reminderArg0s) > 0 {
		queryArg0s := make([]string, len(reminderArg0s))
		for i, arg0 := range reminderArg0s {
			queryArg0s[i] = fmt.Sprintf("vt.reminders %s", arg0)
		}
		attrResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: timedb.TableAssertions,
			Conditions: []*jqlpb.Condition{{
				Requires: []*jqlpb.Filter{
					{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: queryArg0s}}},
				},
			}},
		})
		if err != nil {
			return err
		}
		attrs := map[string]map[string]string{}
		assnPKs := map[string]map[string]string{}
		for _, row := range attrResp.Rows {
			arg0 := strings.TrimPrefix(row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldArg0)].Formatted, "vt.reminders ")
			rel := strings.TrimPrefix(row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldRelation)].Formatted, ".")
			val := row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldArg1)].Formatted
			pk := row.Entries[api.GetPrimary(attrResp.Columns)].Formatted
			if attrs[arg0] == nil {
				attrs[arg0] = map[string]string{}
				assnPKs[arg0] = map[string]string{}
			}
			attrs[arg0][rel] = val
			assnPKs[arg0][rel] = pk
		}
		for _, arg0 := range reminderArg0s {
			a := attrs[arg0]
			taskRef := a["Task"]
			checkText := a["Check"]
			status := a["Status"]
			taskPK := ""
			taskArg0 := ""
			if table, pk := api.ParseForeignKey(taskRef); table == timedb.TableTasks {
				taskPK = pk
				taskArg0 = "tasks " + pk
			} else if strings.HasPrefix(taskRef, "tasks ") {
				taskPK = taskRef[len("tasks "):]
				taskArg0 = taskRef
			}
			desc := checkText
			if desc == "" {
				desc = taskPK
			}
			mv.reminderCache[arg0] = &reminderInfo{
				taskPK:       taskPK,
				taskArg0:     taskArg0,
				checkText:    checkText,
				description:  desc,
				status:       status,
				statusAssnPK: assnPKs[arg0]["Status"],
			}
		}
	}

	// Second pass: build DayItems in order, preserving break structure
	currentBreak := ""
	for _, row := range resp.Rows {
		arg1 := row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		if strings.HasPrefix(arg1, reminderFKPrefix) && strings.HasSuffix(arg1, "}") {
			arg0 := arg1[len(reminderFKPrefix) : len(arg1)-1]
			info, ok := mv.reminderCache[arg0]
			if !ok {
				continue
			}
			prefix := "[ ]"
			switch info.status {
			case "Done":
				prefix = "[x]"
			case "Failed", "Elided":
				prefix = "[-]"
			}
			mv.today = append(mv.today, DayItem{
				Break:        currentBreak,
				Description:  fmt.Sprintf("%s %s", prefix, info.description),
				PK:           reminderArg02EntryPK[arg0],
				ReminderArg0: arg0,
			})
			mv.today2item[arg0] = DayItemMeta{TaskPK: info.taskPK}
		} else {
			currentBreak = arg1
		}
	}
	return nil
}

func (mv *MainView) queryInProgressTasks(ignore string) ([]string, error) {
	tasksTable := mv.tables[timedb.TableTasks]
	tasks, err := mv.queryAllTasks(timedb.StatusActive, timedb.StatusHabitual)
	if err != nil {
		return nil, err
	}
	pks := []string{}
	for _, task := range tasks.Rows {
		pk := task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
		if pk != ignore {
			pks = append(pks, pk)
		}
	}
	return pks, nil
}

func (mv *MainView) queryAllTasks(status ...string) (*jqlpb.ListRowsResponse, error) {
	statusMap := map[string]bool{}
	for _, s := range status {
		statusMap[s] = true
	}
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldStatus,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: status}},
					},
				},
			},
		},
		OrderBy: timedb.FieldDescription,
	})
}

func (mv *MainView) queryLogs(task *jqlpb.Row) (*jqlpb.ListRowsResponse, error) {
	tasksTable := mv.tables[timedb.TableTasks]
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableLog,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldTask,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDescription)].Formatted}},
					},
				},
			},
		},
		OrderBy: timedb.FieldBegin,
		Dec:     true,
	})
}

func (mv *MainView) retrieveAttentionCycle(table *jqlpb.TableMeta, task *jqlpb.Row) (*jqlpb.Row, error) {
	orig := task
	seen := map[string]bool{}
	for {
		pk := task.Entries[api.GetPrimary(table.Columns)].Formatted
		if seen[pk] {
			// hit a cycle
			return orig, nil
		}
		if timedb.IsAttentionCycle(table, task) {
			return task, nil
		}
		seen[pk] = true
		parent := task.Entries[api.IndexOfField(table.Columns, timedb.FieldPrimaryGoal)].Formatted
		resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: timedb.TableTasks,
			Conditions: []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: timedb.FieldDescription,
							Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: parent}},
						},
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Rows) < 1 {
			return orig, nil
		}
		task = resp.Rows[0]
	}
}

func (mv *MainView) switchModes(g *gocui.Gui, v *gocui.View) error {
	mv.justSwitchedGrouping = true
	switch mv.TaskViewMode {
	case TaskViewModeListBar:
		mv.TaskViewMode = TaskViewModeListCycles
	case TaskViewModeListCycles:
		mv.TaskViewMode = TaskViewModeListBar
	}
	return mv.refreshView(g)
}

func (mv *MainView) queryActiveAndHabitualTasks() (*jqlpb.ListRowsResponse, error) {
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldStatus,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: []string{timedb.StatusActive, timedb.StatusHabitual}}},
					},
				},
			},
		},
	})
}

func (mv *MainView) queryPlans(taskPKs []string) (*jqlpb.ListRowsResponse, error) {
	taskCols := []string{}
	for _, task := range taskPKs {
		taskCols = append(taskCols, fmt.Sprintf("tasks %s", task))
	}
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: taskCols}},
					},
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Plan"}},
					},
				},
			},
		},
	})
}

func (mv *MainView) queryDayPlan() (*jqlpb.Row, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Plan"}},
					},
					{
						Column: timedb.FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "today"}},
					},
					{
						Column: timedb.FieldSpan,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Day"}},
					},
					{
						Column: timedb.FieldStatus,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Active"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldStart,
		Dec:     true,
	})

	if err != nil {
		return nil, err
	}
	if len(resp.Rows) == 0 {
		return nil, nil
	}
	return resp.Rows[0], nil
}

func (mv *MainView) queryYesterday() (*jqlpb.Row, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Plan"}},
					},
					{
						Column: timedb.FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "today"}},
					},
					{
						Column: timedb.FieldSpan,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Day"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldStart,
		Dec:     true,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Rows) < 2 {
		return nil, fmt.Errorf("did not find a plan for yesterday")
	}
	return resp.Rows[1], nil
}

// TODO: queryExistingTasks can be deleted once all flows migrate to the new assertion-based reminder model.
func (mv *MainView) queryExistingTasks(planPK string) (map[string]bool, error) {
	assnTable := mv.tables[timedb.TableAssertions]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", planPK)}},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, row := range resp.Rows {
		task := row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		if !isAssertionDayPlan(task) {
			continue
		}
		existing[task] = true
	}
	return existing, nil
}

// TODO: copyOldTasks can be deleted once all flows migrate to the new assertion-based reminder model.
func (mv *MainView) copyOldTasks() error {
	tasksTable := mv.tables[timedb.TableTasks]
	assnTable := mv.tables[timedb.TableAssertions]

	yesterday, err := mv.queryYesterday()
	if err != nil {
		return err
	}
	today, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if today == nil {
		return nil
	}

	todayPK := today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
	yesterdayPK := yesterday.Entries[api.GetPrimary(tasksTable.Columns)].Formatted

	todayBullets, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", todayPK)}},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	// short-circuit if today is already populated
	if len(todayBullets.Rows) > 0 {
		return nil
	}

	oldBullets, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", yesterdayPK)}},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, oldBullet := range oldBullets.Rows {
		rel := oldBullet.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldRelation)].Formatted
		val := oldBullet.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		order := oldBullet.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldOrder)].Formatted

		if isDayTaskDone(val) {
			continue
		}
		// pk doesn't really matter here so using a random integer
		pk := fmt.Sprintf("%d", rand.Int63())
		fields := map[string]string{
			timedb.FieldArg0:      fmt.Sprintf("tasks %s", todayPK),
			timedb.FieldArg1:      val,
			timedb.FieldRelation: rel,
			timedb.FieldOrder:     order,
		}
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:  timedb.TableAssertions,
			Pk:     pk,
			Fields: fields,
		})
		if err != nil {
			return err
		}
	}
	return mv.save()
}

// TODO: refreshToday2Item can be deleted once all flows migrate to the new assertion-based reminder model.
func (mv *MainView) refreshToday2Item() error {
	possibleTaskPKs := []string{}
	activeAndHabitual, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return err
	}
	for _, task := range activeAndHabitual.Rows {
		possibleTaskPKs = append(possibleTaskPKs, task.Entries[api.IndexOfField(activeAndHabitual.Columns, timedb.FieldDescription)].Formatted)
	}

	// In addition to active and habitual tasks we query tasks that were closed
	// recently (and likely after thier corresponding reminders) to try to find where
	// a given reminder came from. The only gap then would be a habitual task (e.g. previous
	// attention cycle) that has since been closed
	for _, item := range mv.today {
		possibleTaskPKs = append(possibleTaskPKs, stripDayPlanPrefix(item.Description))
	}
	matchingTasks, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldDescription,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: possibleTaskPKs}},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	mv.today2item = map[string]DayItemMeta{}
	arg0s := []string{}
	for _, matchingTask := range matchingTasks.Rows {
		taskPK := matchingTask.Entries[api.IndexOfField(matchingTasks.Columns, timedb.FieldDescription)].Formatted
		arg0s = append(arg0s, api.ConstructPolyForeign(timedb.TableTasks, taskPK))
		mv.today2item[taskPK] = DayItemMeta{
			TaskPK: taskPK,
		}
	}
	matchingAssertions, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Plan"}},
					},
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: arg0s}},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	for _, matchingAssertion := range matchingAssertions.Rows {
		assnPK := matchingAssertion.Entries[api.IndexOfField(matchingAssertions.Columns, timedb.FieldDescription)].Formatted
		arg0 := matchingAssertion.Entries[api.IndexOfField(matchingAssertions.Columns, timedb.FieldArg0)]
		arg1 := matchingAssertion.Entries[api.IndexOfField(matchingAssertions.Columns, timedb.FieldArg1)].Formatted
		_, taskPKs := api.ParsePolyforeign(arg0)
		mv.today2item[stripDayPlanPrefix(arg1)] = DayItemMeta{
			AssertionPK: assnPK,
			TaskPK:      taskPKs[0],
		}
	}
	return nil
}

// TODO: queryPossibleDayPlanAdditions can be deleted once all flows migrate to the new assertion-based reminder model.
func (mv *MainView) queryPossibleDayPlanAdditions() ([]string, error) {
	tasksTable := mv.tables[timedb.TableTasks]
	assnTable := mv.tables[timedb.TableAssertions]
	tasks, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return nil, err
	}

	allTasks := []string{}
	task2children := map[string]([]*jqlpb.Row){}
	task2plans := map[string]([]string){}

	for _, task := range tasks.Rows {
		allTasks = append(allTasks, task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)
		parent := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldPrimaryGoal)].Formatted
		task2children[parent] = append(task2children[parent], task)
	}

	plans, err := mv.queryPlans(allTasks)
	if err != nil {
		return nil, err
	}
	descriptions := []string{}
	for _, plan := range plans.Rows {
		planString := plan.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		if isAssertionDayPlan(planString) && !isDayTaskDone(planString) {
			descriptions = append(descriptions, planString)
		}
		task := plan.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg0)].Formatted[len("tasks "):]
		task2plans[task] = append(task2plans[task], planString)
	}
	for _, task := range tasks.Rows {
		pk := task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
		status := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldStatus)].Formatted
		if status != "Active" || len(task2children[pk]) != 0 || len(task2plans[pk]) != 0 {
			continue
		}
		// no need for self reference here
		if !mv.isTaskDayPlan(task) {
			descriptions = append(descriptions, fmt.Sprintf("[ ] %s", pk))
		}
	}
	return descriptions, nil
}

// buildCandidatesForTasks builds (taskPK, checkText) reminder candidates for the given task PKs.
// Only tasks in activePKs receive a bare-task (empty checkText) candidate; all tasks are queried
// for .Check assertions to collect additional check-level candidates.
func (mv *MainView) buildCandidatesForTasks(allPKs []string, activePKs map[string]bool) ([]reminderToPlace, error) {
	arg0s := make([]string, len(allPKs))
	for i, pk := range allPKs {
		arg0s[i] = fmt.Sprintf("tasks %s", pk)
	}
	var checksResp *jqlpb.ListRowsResponse
	var err error
	if len(arg0s) > 0 {
		checksResp, err = mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: timedb.TableAssertions,
			Conditions: []*jqlpb.Condition{{
				Requires: []*jqlpb.Filter{
					{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: arg0s}}},
					{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Check"}}},
				},
			}},
		})
		if err != nil {
			return nil, err
		}
	}
	seenCandidates := map[string]bool{}
	var candidates []reminderToPlace
	for _, pk := range allPKs {
		if !activePKs[pk] {
			continue
		}
		key := pk + "\x00"
		if !seenCandidates[key] {
			seenCandidates[key] = true
			candidates = append(candidates, reminderToPlace{taskPK: pk})
		}
	}
	if checksResp != nil {
		for _, row := range checksResp.Rows {
			arg0 := row.Entries[api.IndexOfField(checksResp.Columns, timedb.FieldArg0)].Formatted
			checkText := row.Entries[api.IndexOfField(checksResp.Columns, timedb.FieldArg1)].Formatted
			taskPK := strings.TrimPrefix(arg0, "tasks ")
			if len(checkText) >= 4 && checkText[0] == '[' && checkText[2] == ']' && checkText[3] == ' ' {
				continue
			}
			key := taskPK + "\x00" + checkText
			if !seenCandidates[key] {
				seenCandidates[key] = true
				candidates = append(candidates, reminderToPlace{taskPK: taskPK, checkText: checkText})
			}
		}
	}
	return candidates, nil
}

// placeRemindersAtPosition resolves candidates via resolveReminderPlacements, filters out reminders
// already in today's plan, shifts existing entries upward to make room, and inserts all new entries
// sequentially starting at insertAfterOrder+1. Returns the count of entries added.
func (mv *MainView) placeRemindersAtPosition(
	dayPlanPK string,
	entries []dayPlanEntry,
	inTodayPlan map[string]bool,
	insertAfterOrder int,
	candidates []reminderToPlace,
	todayStr string,
) (int, error) {
	toCreate, existingToPlace, err := mv.resolveReminderPlacements(candidates)
	if err != nil {
		return 0, err
	}
	var filteredExisting []string
	for _, bareID := range existingToPlace {
		if !inTodayPlan[bareID] {
			filteredExisting = append(filteredExisting, bareID)
		}
	}
	totalToAdd := len(toCreate) + len(filteredExisting)
	if totalToAdd == 0 {
		return 0, nil
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.order > insertAfterOrder {
			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				UpdateOnly: true,
				Table:      timedb.TableAssertions,
				Pk:         e.pk,
				Fields:     map[string]string{timedb.FieldOrder: fmt.Sprintf("%d", e.order+totalToAdd)},
			})
			if err != nil {
				return 0, err
			}
		}
	}
	nextOrder := insertAfterOrder + 1
	for _, bareID := range filteredExisting {
		newPK := fmt.Sprintf("%d", rand.Int63())
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:      timedb.TableAssertions,
			Pk:         newPK,
			InsertOnly: true,
			Fields: map[string]string{
				timedb.FieldRelation: ".Entry",
				timedb.FieldArg0:      fmt.Sprintf("tasks %s", dayPlanPK),
				timedb.FieldArg1:      fmt.Sprintf("@{vt.reminders %s}", bareID),
				timedb.FieldOrder:     fmt.Sprintf("%d", nextOrder),
			},
		})
		if err != nil {
			return 0, err
		}
		nextOrder++
	}
	for _, c := range toCreate {
		if err := mv.createReminderEntity(dayPlanPK, c.taskPK, c.checkText, todayStr, nextOrder); err != nil {
			return 0, err
		}
		nextOrder++
	}
	return totalToAdd, nil
}

// buildCandidatesFromAwaitingReminders returns (taskPK, checkText) candidates for all
// reminder entities whose .Status is Awaiting or Ready. These are fed into
// resolveReminderPlacements so that due reminders surface in the day plan even when their
// parent task is not in the active/habitual query.
// createMissingReminders creates reminder assertion clusters for every (task, check) candidate
// that has no existing reminder entity. Newly created reminders get todayStr as their TargetDate
// and are not yet added to any day plan; addDueRemindersToDay handles that step.
func (mv *MainView) createMissingReminders(candidates []reminderToPlace, todayStr string) error {
	toCreate, _, err := mv.resolveReminderPlacements(candidates)
	if err != nil {
		return err
	}
	for _, c := range toCreate {
		if _, err := mv.createReminder(c.taskPK, c.checkText, todayStr); err != nil {
			return err
		}
	}
	return nil
}

// queryDueReminderBareIDs returns the bare IDs of all Awaiting/Ready reminder entities
// whose TargetDate is at or before todayStr.
func (mv *MainView) queryDueReminderBareIDs(todayStr string) ([]string, error) {
	statusResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Status"}}},
				{Column: timedb.FieldArg1, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: []string{"Awaiting", "Ready"}}}},
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	var bareIDs []string
	var arg0s []string
	for _, row := range statusResp.Rows {
		arg0 := row.Entries[api.IndexOfField(statusResp.Columns, timedb.FieldArg0)].Formatted
		if !strings.HasPrefix(arg0, "vt.reminders ") {
			continue
		}
		bareIDs = append(bareIDs, strings.TrimPrefix(arg0, "vt.reminders "))
		arg0s = append(arg0s, arg0)
	}
	if len(arg0s) == 0 {
		return nil, nil
	}
	tdResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: arg0s}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".TargetDate"}}},
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	targetDates := map[string]string{}
	for _, row := range tdResp.Rows {
		arg0 := row.Entries[api.IndexOfField(tdResp.Columns, timedb.FieldArg0)].Formatted
		arg1 := row.Entries[api.IndexOfField(tdResp.Columns, timedb.FieldArg1)].Formatted
		_, dateStr := api.ParseForeignKey(arg1)
		targetDates[strings.TrimPrefix(arg0, "vt.reminders ")] = dateStr
	}
	var due []string
	for _, bareID := range bareIDs {
		td := targetDates[bareID]
		if td == "" || td <= todayStr {
			due = append(due, bareID)
		}
	}
	return due, nil
}

// addDueRemindersToDay adds all Awaiting/Ready reminders whose TargetDate is today or earlier
// to the day plan, skipping any already present. New entries are inserted after the Zeroeth
// Entry (order 0) if one exists, otherwise at the beginning.
func (mv *MainView) addDueRemindersToDay(dayPlanPK string, entries []dayPlanEntry, inTodayPlan map[string]bool, todayStr string) error {
	dueIDs, err := mv.queryDueReminderBareIDs(todayStr)
	if err != nil {
		return err
	}
	var toAdd []string
	for _, bareID := range dueIDs {
		if !inTodayPlan[bareID] {
			toAdd = append(toAdd, bareID)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	// Resolve task PKs for each due reminder so we can look up habit placement metadata.
	arg0s := make([]string, len(toAdd))
	for i, id := range toAdd {
		arg0s[i] = fmt.Sprintf("vt.reminders %s", id)
	}
	taskResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: arg0s}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Task"}}},
			},
		}},
	})
	if err != nil {
		return err
	}
	taskByBareID := map[string]string{}
	for _, row := range taskResp.Rows {
		arg0 := row.Entries[api.IndexOfField(taskResp.Columns, timedb.FieldArg0)].Formatted
		arg1 := row.Entries[api.IndexOfField(taskResp.Columns, timedb.FieldArg1)].Formatted
		_, taskPK := api.ParseForeignKey(arg1)
		taskByBareID[strings.TrimPrefix(arg0, "vt.reminders ")] = taskPK
	}

	placements := make([]reminderToPlace, len(toAdd))
	seenPKs := map[string]bool{}
	var uniqueTaskPKs []string
	for i, bareID := range toAdd {
		taskPK := taskByBareID[bareID]
		placements[i] = reminderToPlace{taskPK: taskPK}
		if taskPK != "" && !seenPKs[taskPK] {
			seenPKs[taskPK] = true
			uniqueTaskPKs = append(uniqueTaskPKs, taskPK)
		}
	}

	habitMeta, err := mv.fetchHabitPlacementMeta(uniqueTaskPKs)
	if err != nil {
		return err
	}
	for i := range placements {
		if m, ok := habitMeta[placements[i].taskPK]; ok {
			placements[i].dayPlanGroup = m.dayPlanGroup
			placements[i].dayPlanOrder = m.dayPlanOrder
		}
	}

	orderChanges, reminderOrders := computeEntrySequence(entries, placements)
	if err := mv.applyOrderUpdates(orderChanges); err != nil {
		return err
	}
	for i, bareID := range toAdd {
		newPK := fmt.Sprintf("%d", rand.Int63())
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:      timedb.TableAssertions,
			Pk:         newPK,
			InsertOnly: true,
			Fields: map[string]string{
				timedb.FieldRelation: ".Entry",
				timedb.FieldArg0:      fmt.Sprintf("tasks %s", dayPlanPK),
				timedb.FieldArg1:      fmt.Sprintf("@{vt.reminders %s}", bareID),
				timedb.FieldOrder:     fmt.Sprintf("%d", reminderOrders[i]),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) insertNewReminders() error {
	dayPlan, err := mv.queryDayPlan()
	if err != nil || dayPlan == nil {
		return err
	}
	tasksTable := mv.tables[timedb.TableTasks]
	dayPlanPK := dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted

	entries, err := mv.queryDayPlanEntries(dayPlanPK)
	if err != nil {
		return err
	}
	inTodayPlan := map[string]bool{}
	for _, e := range entries {
		if table, pk := api.ParseForeignKey(e.arg1); table == "vt.reminders" {
			inTodayPlan[pk] = true
		}
	}

	activeAndHabitual, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return err
	}
	allPKs := []string{}
	activePKs := map[string]bool{}
	for _, task := range activeAndHabitual.Rows {
		if mv.isTaskDayPlan(task) {
			continue
		}
		pk := task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
		status := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldStatus)].Formatted
		allPKs = append(allPKs, pk)
		if status == timedb.StatusActive {
			activePKs[pk] = true
		}
	}

	todayStr := time.Now().Format("2006-01-02")

	candidates, err := mv.buildCandidatesForTasks(allPKs, activePKs)
	if err != nil {
		return err
	}
	if err := mv.createMissingReminders(candidates, todayStr); err != nil {
		return err
	}
	if err := mv.addDueRemindersToDay(dayPlanPK, entries, inTodayPlan, todayStr); err != nil {
		return err
	}
	return mv.save()
}

// queryDayPlanEntries returns all .Entry assertions on the day plan in order.
func (mv *MainView) queryDayPlanEntries(dayPlanPK string) ([]dayPlanEntry, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", dayPlanPK)}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Entry"}}},
			},
		}},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return nil, err
	}
	entries := make([]dayPlanEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		order, _ := strconv.Atoi(row.Entries[api.IndexOfField(resp.Columns, timedb.FieldOrder)].Formatted)
		entries = append(entries, dayPlanEntry{
			pk:    row.Entries[api.GetPrimary(resp.Columns)].Formatted,
			arg1:  row.Entries[api.IndexOfField(resp.Columns, timedb.FieldArg1)].Formatted,
			order: order,
		})
	}
	return entries, nil
}


// fetchHabitPlacementMeta resolves DayPlanGroup and DayPlanOrder for a set of task PKs
// by following their .Habit assertions to the originating habit tasks (2 batch queries).
func (mv *MainView) fetchHabitPlacementMeta(taskPKs []string) (map[string]habitPlacementMeta, error) {
	if len(taskPKs) == 0 {
		return nil, nil
	}
	taskArg0s := make([]string, len(taskPKs))
	for i, pk := range taskPKs {
		taskArg0s[i] = fmt.Sprintf("tasks %s", pk)
	}
	habitResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: taskArg0s}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Habit"}}},
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	habitToTasks := map[string][]string{}
	habitSeen := map[string]bool{}
	var habitArg0s []string
	for _, row := range habitResp.Rows {
		arg0 := row.Entries[api.IndexOfField(habitResp.Columns, timedb.FieldArg0)].Formatted
		arg1 := row.Entries[api.IndexOfField(habitResp.Columns, timedb.FieldArg1)].Formatted
		taskPK := strings.TrimPrefix(arg0, "tasks ")
		_, habitPK := api.ParseForeignKey(arg1)
		if habitPK == "" {
			continue
		}
		habitToTasks[habitPK] = append(habitToTasks[habitPK], taskPK)
		habitArg0 := fmt.Sprintf("tasks %s", habitPK)
		if !habitSeen[habitArg0] {
			habitSeen[habitArg0] = true
			habitArg0s = append(habitArg0s, habitArg0)
		}
	}
	if len(habitArg0s) == 0 {
		return nil, nil
	}
	for habitPK := range habitToTasks {
		sort.Strings(habitToTasks[habitPK])
	}
	attrResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: habitArg0s}}},
				{Column: timedb.FieldRelation, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: []string{".DayPlanOrder", ".DayPlanGroup"}}}},
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	type ordVal struct {
		ord int
		val string
	}
	habitGroups := map[string][]ordVal{}
	habitOrders := map[string][]ordVal{}
	for _, row := range attrResp.Rows {
		habitPK := strings.TrimPrefix(row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldArg0)].Formatted, "tasks ")
		rel := strings.TrimPrefix(row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldRelation)].Formatted, ".")
		val := row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldArg1)].Formatted
		ord, _ := strconv.Atoi(row.Entries[api.IndexOfField(attrResp.Columns, timedb.FieldOrder)].Formatted)
		switch rel {
		case "DayPlanGroup":
			habitGroups[habitPK] = append(habitGroups[habitPK], ordVal{ord, val})
		case "DayPlanOrder":
			habitOrders[habitPK] = append(habitOrders[habitPK], ordVal{ord, val})
		}
	}
	for habitPK := range habitGroups {
		sort.Slice(habitGroups[habitPK], func(i, j int) bool { return habitGroups[habitPK][i].ord < habitGroups[habitPK][j].ord })
	}
	for habitPK := range habitOrders {
		sort.Slice(habitOrders[habitPK], func(i, j int) bool { return habitOrders[habitPK][i].ord < habitOrders[habitPK][j].ord })
	}
	result := map[string]habitPlacementMeta{}
	for habitPK, tasks := range habitToTasks {
		groups := habitGroups[habitPK]
		orders := habitOrders[habitPK]
		for i, taskPK := range tasks {
			if i >= len(groups) || i >= len(orders) {
				continue
			}
			order, _ := strconv.Atoi(orders[i].val)
			result[taskPK] = habitPlacementMeta{dayPlanGroup: groups[i].val, dayPlanOrder: order}
		}
	}
	return result, nil
}

// computeEntrySequence builds the desired final ordering for existing entries and new
// reminders. Each placement is anchored after the existing entry whose arg1 matches its
// DayPlanGroup (or after the 0th entry if none matches). Placements sharing the same
// anchor are sorted by DayPlanOrder. Returns a map of existing entry pk → new order
// (only changed entries) and a slice of assigned orders parallel to placements.
func computeEntrySequence(entries []dayPlanEntry, placements []reminderToPlace) (map[string]int, []int) {
	reminderOrders := make([]int, len(placements))
	orderChanges := map[string]int{}
	if len(entries) == 0 {
		for i := range placements {
			reminderOrders[i] = i
		}
		return orderChanges, reminderOrders
	}
	textToIdx := map[string]int{}
	for i, e := range entries {
		textToIdx[e.arg1] = i
	}
	type anchoredPlacement struct {
		idx          int
		anchor       int
		dayPlanOrder int
	}
	anchored := make([]anchoredPlacement, len(placements))
	for i, p := range placements {
		anchor := 0
		if p.dayPlanGroup != "" {
			if idx, ok := textToIdx[p.dayPlanGroup]; ok {
				anchor = idx
			}
		}
		anchored[i] = anchoredPlacement{i, anchor, p.dayPlanOrder}
	}
	byAnchor := map[int][]anchoredPlacement{}
	for _, ap := range anchored {
		byAnchor[ap.anchor] = append(byAnchor[ap.anchor], ap)
	}
	for anchor := range byAnchor {
		sort.SliceStable(byAnchor[anchor], func(i, j int) bool {
			return byAnchor[anchor][i].dayPlanOrder < byAnchor[anchor][j].dayPlanOrder
		})
	}
	newOrder := 0
	for i, entry := range entries {
		if entry.order != newOrder {
			orderChanges[entry.pk] = newOrder
		}
		newOrder++
		for _, ap := range byAnchor[i] {
			reminderOrders[ap.idx] = newOrder
			newOrder++
		}
	}
	return orderChanges, reminderOrders
}

// applyOrderUpdates writes changed Order values to existing .Entry assertions.
func (mv *MainView) applyOrderUpdates(orderChanges map[string]int) error {
	for pk, newOrder := range orderChanges {
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      timedb.TableAssertions,
			Pk:         pk,
			Fields:     map[string]string{timedb.FieldOrder: fmt.Sprintf("%d", newOrder)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// resolveReminderPlacements classifies candidates into those needing a new reminder entity
// (toCreate) and bare IDs of existing reminders whose TargetDate is today or earlier
// (existingToPlace). Uses one query to find existing reminders and a second to check
// their TargetDate assertions.
func (mv *MainView) resolveReminderPlacements(candidates []reminderToPlace) (toCreate []reminderToPlace, existingToPlace []string, err error) {
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	arg1Set := map[string]bool{}
	for _, c := range candidates {
		arg1Set[fmt.Sprintf("@{tasks %s}", c.taskPK)] = true
		if c.checkText != "" {
			arg1Set[c.checkText] = true
		}
	}
	arg1Values := make([]string, 0, len(arg1Set))
	for v := range arg1Set {
		arg1Values = append(arg1Values, v)
	}
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{{
			Requires: []*jqlpb.Filter{
				{Column: timedb.FieldArg1, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: arg1Values}}},
			},
		}},
	})
	if err != nil {
		return nil, nil, err
	}
	// Group Arg1 values by reminder ID (Arg0 prefixed "vt.reminders ").
	reminderArg1s := map[string]map[string]bool{}
	for _, row := range resp.Rows {
		arg0 := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldArg0)].Formatted
		if !strings.HasPrefix(arg0, "vt.reminders ") {
			continue
		}
		arg1 := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldArg1)].Formatted
		if reminderArg1s[arg0] == nil {
			reminderArg1s[arg0] = map[string]bool{}
		}
		reminderArg1s[arg0][arg1] = true
	}
	// Index taskRef → reminder IDs.
	taskRefToReminders := map[string][]string{}
	for reminderID, arg1s := range reminderArg1s {
		for arg1 := range arg1s {
			if strings.HasPrefix(arg1, "@{tasks ") {
				taskRefToReminders[arg1] = append(taskRefToReminders[arg1], reminderID)
			}
		}
	}
	// Map each candidate key to the reminder ID that covers it.
	candidateToReminder := map[string]string{}
	for _, c := range candidates {
		taskRef := fmt.Sprintf("@{tasks %s}", c.taskPK)
		key := c.taskPK + "\x00" + c.checkText
		for _, reminderID := range taskRefToReminders[taskRef] {
			if c.checkText == "" || reminderArg1s[reminderID][c.checkText] {
				candidateToReminder[key] = reminderID
				break
			}
		}
	}
	// Query TargetDate for all matched reminder IDs.
	seenIDs := map[string]bool{}
	var matchedIDs []string
	for _, reminderID := range candidateToReminder {
		if !seenIDs[reminderID] {
			seenIDs[reminderID] = true
			matchedIDs = append(matchedIDs, reminderID)
		}
	}
	reminderTargetDate := map[string]string{} // reminderID → "YYYY-MM-DD" or ""
	if len(matchedIDs) > 0 {
		tdResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: timedb.TableAssertions,
			Conditions: []*jqlpb.Condition{{
				Requires: []*jqlpb.Filter{
					{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: matchedIDs}}},
					{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".TargetDate"}}},
				},
			}},
		})
		if err != nil {
			return nil, nil, err
		}
		for _, row := range tdResp.Rows {
			arg0 := row.Entries[api.IndexOfField(tdResp.Columns, timedb.FieldArg0)].Formatted
			val := row.Entries[api.IndexOfField(tdResp.Columns, timedb.FieldArg1)].Formatted
			_, dateStr := api.ParseForeignKey(val)
			reminderTargetDate[arg0] = dateStr
		}
	}
	todayStr := time.Now().Format("2006-01-02")
	existingSet := map[string]bool{}
	for _, c := range candidates {
		key := c.taskPK + "\x00" + c.checkText
		reminderID, exists := candidateToReminder[key]
		if !exists {
			toCreate = append(toCreate, c)
			continue
		}
		targetDate := reminderTargetDate[reminderID]
		if targetDate == "" || targetDate <= todayStr {
			bareID := strings.TrimPrefix(reminderID, "vt.reminders ")
			if !existingSet[bareID] {
				existingSet[bareID] = true
				existingToPlace = append(existingToPlace, bareID)
			}
		}
	}
	return toCreate, existingToPlace, nil
}

// createReminder creates the assertion cluster for a new reminder and returns its bare ID.
// It does NOT add the reminder to any day plan; use createReminderEntity for that.
func (mv *MainView) createReminder(taskPK, checkText, targetDate string) (string, error) {
	bareID := fmt.Sprintf("%d", rand.Int63())
	reminderRef := fmt.Sprintf("vt.reminders %s", bareID)
	assns := []map[string]string{
		{timedb.FieldRelation: ".Status", timedb.FieldArg0: reminderRef, timedb.FieldArg1: "Awaiting"},
		{timedb.FieldRelation: ".Task", timedb.FieldArg0: reminderRef, timedb.FieldArg1: fmt.Sprintf("@{tasks %s}", taskPK)},
		{timedb.FieldRelation: ".TargetDate", timedb.FieldArg0: reminderRef, timedb.FieldArg1: fmt.Sprintf("@{dates %s}", targetDate)},
	}
	if checkText != "" {
		assns = append(assns, map[string]string{timedb.FieldRelation: ".Check", timedb.FieldArg0: reminderRef, timedb.FieldArg1: checkText})
	}
	for _, fields := range assns {
		pk := fmt.Sprintf("%d", rand.Int63())
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:      timedb.TableAssertions,
			Pk:         pk,
			InsertOnly: true,
			Fields:     fields,
		})
		if err != nil {
			return "", err
		}
	}
	return bareID, nil
}

func (mv *MainView) createReminderEntity(dayPlanPK, taskPK, checkText, targetDate string, entryOrder int) error {
	bareID, err := mv.createReminder(taskPK, checkText, targetDate)
	if err != nil {
		return err
	}
	entryPK := fmt.Sprintf("%d", rand.Int63())
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      timedb.TableAssertions,
		Pk:         entryPK,
		InsertOnly: true,
		Fields: map[string]string{
			timedb.FieldRelation: ".Entry",
			timedb.FieldArg0:      fmt.Sprintf("tasks %s", dayPlanPK),
			timedb.FieldArg1:      fmt.Sprintf("@{vt.reminders %s}", bareID),
			timedb.FieldOrder:     fmt.Sprintf("%d", entryOrder),
		},
	})
	return err
}

// maybeMarkPreviousDayPlanSatisfied marks the current active day plan as Satisfied if its start
// date is before today, closing it out before autofill creates a fresh one.
func (mv *MainView) maybeMarkPreviousDayPlanSatisfied() error {
	dayPlan, err := mv.queryDayPlan()
	if err != nil || dayPlan == nil {
		return err
	}
	tasksTable := mv.tables[timedb.TableTasks]
	startStr := dayPlan.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldStart)].Formatted
	startDate, err := api.ParseFormattedDate(startStr)
	if err != nil {
		return nil // unparseable date — skip rather than error
	}
	today := time.Now().Truncate(24 * time.Hour)
	if !startDate.Before(today) {
		return nil
	}
	pk := dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		UpdateOnly: true,
		Table:      timedb.TableTasks,
		Pk:         pk,
		Fields:     map[string]string{timedb.FieldStatus: timedb.StatusSatisfied},
	})
	return err
}

func (mv *MainView) refreshTasks(g *gocui.Gui, v *gocui.View) error {
	if err := mv.maybeMarkPreviousDayPlanSatisfied(); err != nil {
		return err
	}
	_, err := api.RunMacro(ctx, mv.dbms, "jql-timedb-autofill --v2", api.MacroCurrentView{}, true)
	if err != nil {
		return err
	}
	if err := mv.carryForwardEntries(); err != nil {
		return err
	}
	if err = mv.load(g); err != nil {
		return err
	}
	err = mv.insertNewReminders()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

// carryForwardEntries copies .Entry assertions from yesterday's day plan to today's,
// skipping any reminder references whose status is Done, Failed, or Elided.
func (mv *MainView) carryForwardEntries() error {
	tasksTable := mv.tables[timedb.TableTasks]

	yesterday, err := mv.queryYesterday()
	if err != nil {
		return nil // no yesterday plan — nothing to carry forward
	}
	today, err := mv.queryDayPlan()
	if err != nil || today == nil {
		return err
	}

	todayPK := today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
	yesterdayPK := yesterday.Entries[api.GetPrimary(tasksTable.Columns)].Formatted

	// Short-circuit if today already has entries.
	todayEntries, err := mv.queryDayPlanEntries(todayPK)
	if err != nil {
		return err
	}
	if len(todayEntries) > 0 {
		return nil
	}

	// Get yesterday's entries in order.
	yesterdayEntries, err := mv.queryDayPlanEntries(yesterdayPK)
	if err != nil {
		return err
	}
	if len(yesterdayEntries) == 0 {
		return nil
	}

	// Collect reminder PKs so we can batch-query their statuses.
	var reminderPKs []string
	for _, e := range yesterdayEntries {
		if table, pk := api.ParseForeignKey(e.arg1); table == "vt.reminders" {
			reminderPKs = append(reminderPKs, fmt.Sprintf("vt.reminders %s", pk))
		}
	}

	// Build a set of reminder PKs that should be skipped (Done/Failed/Elided).
	skipReminders := map[string]bool{}
	if len(reminderPKs) > 0 {
		statusResp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: timedb.TableAssertions,
			Conditions: []*jqlpb.Condition{{
				Requires: []*jqlpb.Filter{
					{Column: timedb.FieldArg0, Match: &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: reminderPKs}}},
					{Column: timedb.FieldRelation, Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Status"}}},
				},
			}},
		})
		if err != nil {
			return err
		}
		for _, row := range statusResp.Rows {
			arg0 := row.Entries[api.IndexOfField(statusResp.Columns, timedb.FieldArg0)].Formatted
			status := row.Entries[api.IndexOfField(statusResp.Columns, timedb.FieldArg1)].Formatted
			switch status {
			case "Done", "Failed", "Elided":
				skipReminders[arg0] = true
			}
		}
	}

	// Copy qualifying entries to today's plan, preserving order.
	for _, e := range yesterdayEntries {
		if table, pk := api.ParseForeignKey(e.arg1); table == "vt.reminders" {
			if skipReminders[fmt.Sprintf("vt.reminders %s", pk)] {
				continue
			}
		}
		newPK := fmt.Sprintf("%d", rand.Int63())
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:      timedb.TableAssertions,
			Pk:         newPK,
			InsertOnly: true,
			Fields: map[string]string{
				timedb.FieldRelation: ".Entry",
				timedb.FieldArg0:      fmt.Sprintf("tasks %s", todayPK),
				timedb.FieldArg1:      e.arg1,
				timedb.FieldOrder:     fmt.Sprintf("%d", e.order),
			},
		})
		if err != nil {
			return err
		}
	}
	return mv.save()
}

func (mv *MainView) taskMarker(status string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return mv.markTask(g, v, status)
	}
}

func (mv *MainView) markTask(g *gocui.Gui, v *gocui.View, status string) error {
	if mv.span != timedb.Today {
		return nil
	}
	tasksView, err := g.View(timedb.TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	if ix >= len(mv.cachedTodayTasks) {
		return nil
	}
	item, ok := mv.ix2item[ix]
	if !ok || item.ReminderArg0 == "" {
		return nil
	}
	info, ok := mv.reminderCache[item.ReminderArg0]
	if !ok {
		return nil
	}
	reminderStatus := "Awaiting"
	if status == timedb.StatusSatisfied {
		reminderStatus = "Done"
	} else if status == timedb.StatusFailed || status == timedb.StatusAbandoned {
		reminderStatus = "Failed"
	}
	if info.statusAssnPK != "" {
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      timedb.TableAssertions,
			Pk:         info.statusAssnPK,
			Fields:     map[string]string{timedb.FieldArg1: reminderStatus},
		})
	} else {
		pk := fmt.Sprintf("%d", rand.Int63())
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			InsertOnly: true,
			Table:      timedb.TableAssertions,
			Pk:         pk,
			Fields: map[string]string{
				timedb.FieldRelation: ".Status",
				timedb.FieldArg0:      fmt.Sprintf("vt.reminders %s", item.ReminderArg0),
				timedb.FieldArg1:      reminderStatus,
			},
		})
	}
	if err != nil {
		return err
	}
	if info.taskPK != "" && status != "" {
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      timedb.TableTasks,
			Pk:         info.taskPK,
			Fields:     map[string]string{timedb.FieldStatus: status},
		})
		if err != nil {
			return err
		}
	}
	err = mv.save()
	if err != nil {
		return err
	}
	err = mv.cursorDown(g, v)
	if err != nil {
		return err
	}
	err = mv.refreshView(g)
	if err != nil {
		return err
	}
	err = mv.possiblyPromptForNextNounState(info.taskPK)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) possiblyPromptForNextNounState(taskPK string) error {
	task, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableTasks,
		Pk:    taskPK,
	})
	if err != nil {
		return err
	}
	nounPK := task.Row.Entries[api.IndexOfField(task.Columns, timedb.FieldDirect)].Formatted
	noun, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableNouns,
		Pk:    nounPK,
	})
	if err != nil {
		if api.IsNotExistError(err) {
			return nil
		}
		return err
	}
	status := noun.Row.Entries[api.IndexOfField(noun.Columns, timedb.FieldStatus)].Formatted
	nextStates := getNextNounStates()
	next, ok := nextStates[status]
	if ok {
		mv.MainViewMode = MainViewModeQueryingForNounNextState
		mv.nounSwitchingStatePK = nounPK
		mv.nounStateNextState = next
	}
	return nil
}

func getNextNounStates() map[string]string {
	return map[string]string{
		timedb.StatusIdea:         timedb.StatusExploring,
		timedb.StatusExploring:    timedb.StatusPlanning,
		timedb.StatusPlanning:     timedb.StatusImplementing,
		timedb.StatusImplementing: timedb.StatusRevisit,
	}
}

func (mv *MainView) deleteDayPlan(g *gocui.Gui, v *gocui.View) error {
	if mv.span != timedb.Today {
		return nil
	}
	tasksView, err := g.View(timedb.TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	_, err = mv.dbms.DeleteRow(ctx, &jqlpb.DeleteRowRequest{
		Table: timedb.TableAssertions,
		Pk:    item.PK,
	})
	if err != nil {
		return err
	}
	err = mv.save()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

type CurrentDomainInfo struct {
	Skillset   string
	Direct     string
	TaskPK     string
	IsPrepTask bool
	IsWarmup   bool
}

func (mv *MainView) GetCurrentDomain(g *gocui.Gui, v *gocui.View) (CurrentDomainInfo, error) {
	tasksTable := mv.tables[timedb.TableTasks]
	taskPk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return CurrentDomainInfo{}, err
	}
	resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableTasks,
		Pk:    taskPk,
	})
	if err != nil {
		return CurrentDomainInfo{}, err
	}
	action := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldAction)].Formatted
	direct := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDirect)].Formatted
	indirect := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldIndirect)].Formatted
	isPrepareTask := (direct == "" && indirect == "")
	isWarmup := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldAction)].Formatted == "Warm-up"
	skillset := direct
	if action != "Practice" {
		cycle, err := mv.retrieveAttentionCycle(tasksTable, resp.Row)
		if err != nil {
			return CurrentDomainInfo{}, err
		}
		skillset = cycle.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldIndirect)].Formatted
	}
	return CurrentDomainInfo{
		IsPrepTask: isPrepareTask,
		Direct:     direct,
		Skillset:   skillset,
		TaskPK:     taskPk,
		IsWarmup:   isWarmup,
	}, nil
}

func (mv *MainView) InjectTaskWithAllMatching(g *gocui.Gui, v *gocui.View, matchAttentionCycle bool) (int, error) {
	// Return the count of added items so that a higher level caller can decide to redirect
	// the user to populate new items or not
	tasksTable := mv.tables[timedb.TableTasks]
	taskPk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return 0, err
	}
	filters := []*jqlpb.Filter{
		{
			Column: timedb.FieldStatus,
			Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: timedb.StatusActive}},
		},
	}
	if matchAttentionCycle {
		resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
			Table: timedb.TableTasks,
			Pk:    taskPk,
		})
		if err != nil {
			return 0, err
		}
		cycle, err := mv.retrieveAttentionCycle(tasksTable, resp.Row)
		if err != nil {
			return 0, err
		}
		cycleName := cycle.Entries[api.GetPrimary(mv.tables[timedb.TableTasks].Columns)].Formatted
		filters = append(filters, &jqlpb.Filter{
			Column: timedb.FieldPrimaryGoal,
			Match:  &jqlpb.Filter_PathToMatch{&jqlpb.PathToMatch{Value: cycleName}},
		})
	}

	activeDescendants, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: filters,
			},
		},
	})
	if err != nil {
		return 0, err
	}
	allPKs := []string{}
	activePKs := map[string]bool{}
	for _, row := range activeDescendants.Rows {
		action := row.Entries[api.IndexOfField(mv.tables[timedb.TableTasks].Columns, timedb.FieldAction)].Formatted
		direct := row.Entries[api.IndexOfField(mv.tables[timedb.TableTasks].Columns, timedb.FieldDirect)].Formatted
		indirect := row.Entries[api.IndexOfField(mv.tables[timedb.TableTasks].Columns, timedb.FieldIndirect)].Formatted
		if action == "Plan" && direct == "today" && indirect == "" {
			continue
		}
		pk := row.Entries[api.GetPrimary(mv.tables[timedb.TableTasks].Columns)].Formatted
		allPKs = append(allPKs, pk)
		activePKs[pk] = true
	}

	dayPlan, err := mv.queryDayPlan()
	if err != nil || dayPlan == nil {
		return 0, err
	}
	dayPlanPK := dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
	todayStr := time.Now().Format("2006-01-02")

	entries, err := mv.queryDayPlanEntries(dayPlanPK)
	if err != nil {
		return 0, err
	}
	inTodayPlan := map[string]bool{}
	for _, e := range entries {
		if table, pk := api.ParseForeignKey(e.arg1); table == "vt.reminders" {
			inTodayPlan[pk] = true
		}
	}

	// Find the order of the entry under the cursor to use as insertion point.
	var cursorEntryPK string
	if tasksView, viewErr := g.View(timedb.TasksView); viewErr == nil {
		_, oy := tasksView.Origin()
		_, cy := tasksView.Cursor()
		if item, ok := mv.ix2item[oy+cy]; ok {
			cursorEntryPK = item.PK
		}
	}
	insertAfterOrder := -1
	for _, e := range entries {
		if e.pk == cursorEntryPK {
			insertAfterOrder = e.order
			break
		}
	}
	if insertAfterOrder == -1 && len(entries) > 0 {
		insertAfterOrder = entries[len(entries)-1].order
	}

	candidates, err := mv.buildCandidatesForTasks(allPKs, activePKs)
	if err != nil {
		return 0, err
	}

	added, err := mv.placeRemindersAtPosition(dayPlanPK, entries, inTodayPlan, insertAfterOrder, candidates, todayStr)
	if err != nil {
		return added, err
	}
	if added > 0 {
		if err = mv.save(); err != nil {
			return added, err
		}
	}
	return added, mv.refreshView(g)
}

// TODO: queryPresentAndFutureDayPlanNames can be deleted once all flows migrate to the new assertion-based reminder model.
func (mv *MainView) queryPresentAndFutureDayPlanNames() (map[string]bool, error) {
	today, err := mv.queryDayPlan()
	if err != nil {
		return nil, err
	}
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)}},
					},
				},
			},
		},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, row := range resp.Rows {
		arg1 := row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		if !isAssertionDayPlan(arg1) {
			continue
		}
		names[stripDayPlanPrefix(arg1)] = true
	}
	return names, nil
}

func (mv *MainView) substituteTaskWithPrompt(g *gocui.Gui, v *gocui.View) error {
	if mv.span != timedb.Today {
		return nil
	}
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	if item.ReminderArg0 != "" {
		if info, ok := mv.reminderCache[item.ReminderArg0]; ok && info.taskPK != "" {
			return mv.substituteTaskWithPlans(g, info.taskPK)
		}
	}
	return nil
}

func (mv *MainView) substituteTaskWithPlans(g *gocui.Gui, taskPK string) error {
	mv.substitutingPlan = false
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	task, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: timedb.TableTasks,
		Pk:    taskPK,
	})
	if err != nil {
		return err
	}
	direct := task.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDirect)].Formatted
	action := task.Row.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldAction)].Formatted
	procedures, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "nouns " + direct}},
					},
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Procedure"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return err
	}
	// TODO this probably has a lot in common with logic in the procedure navigator
	// so should be made into a shared library
	procedure := ""
	prefix := fmt.Sprintf("### %s\n", action)
	for _, proc := range procedures.Rows {
		procText := proc.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg1)].Formatted
		if strings.HasPrefix(procText, prefix) {
			procedure = strings.TrimSpace(procText[len(prefix):])
			break
		}
	}
	items := []PlanSelectionItem{}
	for _, item := range strings.Split(procedure, "\n") {
		if !strings.HasPrefix(item, "- ") {
			continue
		}
		items = append(items, PlanSelectionItem{
			Plan:   item[2:],
			Marked: false,
		})
	}
	mv.planSelections = items
	mv.MainViewMode = MainViewModeQueryingForPlansSubset
	return mv.refreshView(g)
}

func (mv *MainView) substitutePlanWithImplementation(g *gocui.Gui, plan string) error {
	mv.substitutingPlan = true
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	candidates, err := mv.queryAllTasks(timedb.StatusActive, timedb.StatusHabitual, timedb.StatusPlanned, timedb.StatusPending)
	if err != nil {
		return err
	}
	candidatePKs := map[string]bool{}
	for _, candidate := range candidates.Rows {
		candidatePKs[candidate.Entries[api.GetPrimary(tasksTable.Columns)].Formatted] = true
	}
	implementations, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldArg1,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: plan}},
					},
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Implements"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return err
	}
	items := []PlanSelectionItem{}
	for _, row := range implementations.Rows {
		pk := row.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg0)].Formatted[len("tasks "):]
		if !candidatePKs[pk] {
			continue
		}
		items = append(items, PlanSelectionItem{
			Plan:   pk,
			Marked: false,
		})
	}
	mv.planSelections = items
	mv.MainViewMode = MainViewModeQueryingForPlansSubset
	return mv.refreshView(g)
}

func (mv *MainView) queryForPlanSubsetLayout(g *gocui.Gui) error {
	maxX, _ := g.Size()
	newPlansView, err := g.SetView(timedb.NewPlansView, 4, 5, maxX-4, len(mv.planSelections)+8)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	newPlansView.Editable = true
	newPlansView.Highlight = true
	newPlansView.SelBgColor = gocui.ColorWhite
	newPlansView.SelFgColor = gocui.ColorBlack
	newPlansView.Editor = mv
	g.SetCurrentView(timedb.NewPlansView)
	newPlansView.Clear()
	newPlansView.Write([]byte("Select your plans\n"))
	for _, item := range mv.planSelections {
		if item.Marked {
			newPlansView.Write([]byte("[x] "))
		} else {
			newPlansView.Write([]byte("[ ] "))
		}
		newPlansView.Write([]byte(item.Plan + "\n"))
	}
	return nil
}

func (mv *MainView) queryForNounNextStateLayout(g *gocui.Gui) error {
	maxX, _ := g.Size()
	nextStateView, err := g.SetView(timedb.NextStateView, 4, 5, maxX-4, 12)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	nextStateView.Highlight = true
	nextStateView.SelBgColor = gocui.ColorWhite
	nextStateView.SelFgColor = gocui.ColorBlack
	g.SetCurrentView(timedb.NextStateView)
	nextStateView.Clear()
	nextStateView.Write([]byte(fmt.Sprintf("Keep %q as is\n", mv.nounSwitchingStatePK)))
	nextStateView.Write([]byte(fmt.Sprintf("Mark %q\n", mv.nounStateNextState)))
	nextStateView.Write([]byte(fmt.Sprintf("Mark %q\n", timedb.StatusSatisfied)))
	nextStateView.Write([]byte(fmt.Sprintf("Mark %q", timedb.StatusHabitual)))
	return nil
}

func (mv *MainView) selectNextNounState(g *gocui.Gui, v *gocui.View) error {
	_, y := v.Cursor()
	// values are based on the values written to the prompt in queryForNounNextStateLayout
	nextState := ""
	switch y {
	case 1:
		nextState = mv.nounStateNextState
	case 2:
		nextState = timedb.StatusSatisfied
	case 3:
		nextState = timedb.StatusHabitual
	}
	err := g.DeleteView(timedb.NextStateView)
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	if nextState == "" {
		return nil
	}
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table: timedb.TableNouns,
		Pk:    mv.nounSwitchingStatePK,
		Fields: map[string]string{
			timedb.FieldStatus: nextState,
		},
		UpdateOnly: true,
	})
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) wrapTaskInRamps(g *gocui.Gui, v *gocui.View) error {
	if mv.span != timedb.Today || mv.MainViewMode != MainViewModeListBar {
		return nil
	}
	pk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return err
	}
	for _, action := range []string{"Prepare", "Wrap-up"} {
		fields := map[string]string{
			timedb.FieldAction:      action,
			timedb.FieldPrimaryGoal: pk,
			timedb.FieldStart:       "",
			timedb.FieldStatus:      timedb.StatusActive,
		}
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:  timedb.TableTasks,
			Pk:     "",
			Fields: fields,
		})
		if err != nil {
			return err
		}
		view := api.MacroCurrentView{
			Table:            timedb.TableTasks,
			PrimarySelection: "",
		}
		_, err = api.RunMacro(ctx, mv.dbms, "jql-timedb-setpk --v2", view, true)
		if err != nil {
			return err
		}
	}
	created, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldPrimaryGoal,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: pk}},
					},
					{
						Column: timedb.FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
					},
					{
						Column: timedb.FieldIndirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
					},
				},
			},
		},
		OrderBy: timedb.FieldAction,
		Dec:     true,
	})
	if err != nil {
		return err
	}
	primary := api.GetPrimary(created.Columns)
	for i, row := range created.Rows {
		pk := row.Entries[primary].Formatted
		delta := 0
		if i > 0 {
			delta = -1 // We want to wrap the task so the first new task should come before it
		}
		err = mv.insertDayPlan(g, pk, delta)
		if err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) toggleAllPlans(g *gocui.Gui, v *gocui.View) error {
	// if any are unmarked we want to mark everything, otherwise we mark nothing
	allMarked := true
	for _, sel := range mv.planSelections {
		allMarked = allMarked && sel.Marked
	}

	for i := range mv.planSelections {
		mv.planSelections[i].Marked = !allMarked
	}
	return mv.refreshView(g)
}

func (mv *MainView) markPlanSelection(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	// HACK we know we have a one-line title bar here
	mv.planSelections[cy+oy-1].Marked = !(mv.planSelections[cy+oy-1].Marked)
	return mv.refreshView(g)
}

func (mv *MainView) substitutePlanSelections(g *gocui.Gui, v *gocui.View) error {
	if mv.substitutingPlan {
		return mv.substitutePlanSelectionsForPlan(g, v)
	} else {
		return mv.substitutePlanSelectionsForTask(g, v)
	}
}

func (mv *MainView) substitutePlanSelectionsForPlan(g *gocui.Gui, v *gocui.View) error {
	err := g.DeleteView(timedb.NewPlansView)
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	inserted := false
	updated := []string{}
	for _, item := range mv.planSelections {
		if !item.Marked {
			continue
		}
		inserted = true
		taskPK := item.Plan
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table: timedb.TableTasks,
			Pk:    taskPK,
			Fields: map[string]string{
				timedb.FieldSpan:   "Day",
				timedb.FieldStart:  "",
				timedb.FieldStatus: "Active",
			},
			UpdateOnly: true,
		})
		if err != nil {
			return err
		}
		updated = append(updated, taskPK)
		err = mv.insertDayPlan(g, item.Plan, 0)
		if err != nil {
			return err
		}
	}
	// If the user didn't mark any selections then don't actually change anything
	if !inserted {
		return nil
	}
	// NOTE we rely on markTask to also save our changes
	err = mv.markTask(g, v, timedb.StatusSatisfied)
	if err != nil {
		return err
	}
	err = mv.syncPKs(timedb.TableTasks, updated)
	if err != nil {
		return err
	}
	// NOTE we rely on deleteDayPlan to also save our changes
	err = mv.deleteDayPlan(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) substitutePlanSelectionsForTask(g *gocui.Gui, v *gocui.View) error {
	tasksView, err := g.View(timedb.TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	meta := mv.today2item[item.ReminderArg0]
	// insert in reverse order since insertion is to the beginning
	for i := len(mv.planSelections) - 1; i >= 0; i-- {
		item := mv.planSelections[i]
		if item.Marked {
			err := mv.createNewPlan(g, meta.TaskPK, item.Plan)
			if err != nil {
				return err
			}
		}
	}
	err = g.DeleteView(timedb.NewPlansView)
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	return mv.refreshView(g)
}

func (mv *MainView) toggleFocus(g *gocui.Gui, v *gocui.View) error {
	mv.focusing = !mv.focusing
	mv.justSwitchedGrouping = true
	return mv.refreshView(g)
}

// queryPendingNoImplements will query for pending tasks that don't have an
// Implements attribute. Tasks which implement other tasks are noisy so
// shouldn't be shown in an overview pane and will get picked up otherwise
// anyway.
func (mv *MainView) queryPendingNoImplements() ([]*jqlpb.Row, error) {
	assnTable := mv.tables[timedb.TableAssertions]
	tasksTable := mv.tables[timedb.TableTasks]
	pending, err := mv.queryAllTasks(timedb.StatusPending)
	if err != nil {
		return nil, err
	}
	pk2task := map[string](*jqlpb.Row){}
	for _, task := range pending.Rows {
		pk2task[task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted] = task
	}
	implementations, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: timedb.TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Implements"}},
					},
				},
			},
		},
		OrderBy: timedb.FieldOrder,
	})
	if err != nil {
		return nil, err
	}
	for _, assn := range implementations.Rows {
		obj := assn.Entries[api.IndexOfField(assnTable.Columns, timedb.FieldArg0)]
		if !strings.HasPrefix(obj.Formatted, "tasks ") {
			continue
		}
		pk := obj.Formatted[len("tasks "):]
		delete(pk2task, pk)
	}

	sorted := make([]string, 0, len(pk2task))
	for pk := range pk2task {
		sorted = append(sorted, pk)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	rows := make([]*jqlpb.Row, 0, len(sorted))

	for _, pk := range sorted {
		rows = append(rows, pk2task[pk])
	}
	return rows, nil
}

func isAssertionDayPlan(description string) bool {
	return isDayTaskDone(description) || strings.HasPrefix(description, "[ ] ")
}

func isDayTaskDone(description string) bool {
	return strings.HasPrefix(description, "[x] ") || strings.HasPrefix(description, "[-] ")
}

func taskToDayPlan(description string) string {
	return "[ ] " + description
}

func stripDayPlanPrefix(s string) string {
	return s[len("[ ] "):]
}

func (mv *MainView) isTaskDayPlan(task *jqlpb.Row) bool {
	tasksTable := mv.tables[timedb.TableTasks]
	action := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldAction)].Formatted
	direct := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldDirect)].Formatted
	indirect := task.Entries[api.IndexOfField(tasksTable.Columns, timedb.FieldIndirect)].Formatted
	return action == "Plan" && direct == "today" && indirect == ""
}
