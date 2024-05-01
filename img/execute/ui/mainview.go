package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
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
	today2item       map[string]DayItemMeta
	ix2item          map[int]DayItem

	// state used for searching tasks
	topicQ          string
	unfilteredTasks []string
	filteredTasks   []string
	queryCallback   func(taskPK string) error

	// state used for querying for a new plan
	newPlanTaskPK      string
	newPlanDescription string

	// state used for querying for a subset of plans
	planSelections   []PlanSelectionItem
	substitutingPlan bool

	// state used for focus mode
	focusing             bool
	justSwitchedGrouping bool
}

type DayItem struct {
	Break       string
	Description string
	PK          string
}

type DayItemMeta struct {
	Description string
	TaskPK      string
	AssertionPK string
}

type PlanSelectionItem struct {
	Plan   string
	Marked bool
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, dbms api.JQL_DBMS) (*MainView, error) {
	rand.Seed(time.Now().UnixNano())
	mv := &MainView{
		dbms: dbms,
	}
	return mv, mv.load(g)
}

func (mv *MainView) load(g *gocui.Gui) error {
	mv.MainViewMode = MainViewModeListBar
	mv.tasks = map[string]([]*jqlpb.Row){}
	mv.span = Today
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
	err := g.DeleteView(QueryTasksView)
	if err != nil {
		return err
	}
	err = g.DeleteView(QueryView)
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
	} else {
		return mv.listTasksLayout(g)
	}
}

func (mv *MainView) createNewPlanFromInput(g *gocui.Gui, v *gocui.View) error {
	err := g.DeleteView(NewPlanView)
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	err = mv.createNewPlan(g, mv.newPlanTaskPK, mv.newPlanDescription)
	if err != nil {
		return err
	}
	mv.newPlanTaskPK = ""
	mv.newPlanDescription = ""
	return nil
}

func (mv *MainView) createNewPlan(g *gocui.Gui, taskPK, description string) error {
	assnTable := mv.tables[TableAssertions]
	newOrder := 0
	plansResp, err := mv.queryPlans([]string{taskPK})
	if err != nil {
		return err
	}
	for _, plan := range plansResp.Rows {
		orderInt, err := strconv.Atoi(plan.Entries[api.IndexOfField(assnTable.Columns, FieldOrder)].Formatted)
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
		FieldArg0:      fmt.Sprintf("tasks %s", taskPK),
		FieldArg1:      fmt.Sprintf("[ ] %s", description),
		FieldARelation: ".Plan",
		FieldOrder:     fmt.Sprintf("%d", newOrder),
	}
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  TableAssertions,
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

func (mv *MainView) insertDayPlan(g *gocui.Gui, description string, delta int) error {
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	tasksView, err := g.View(TasksView)
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
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: dayPlanLink}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Do Today"}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return err
	}
	dayOrder := 0
	for _, row := range existingTodos.Rows {
		if row.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted == insertsAfter.Description {
			dayOrder, err = strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, FieldOrder)].Formatted)
			if err != nil {
				return err
			}
		}
	}
	dayOrder += delta
	updated := []string{}
	for _, row := range existingTodos.Rows {
		orderInt, err := strconv.Atoi(row.Entries[api.IndexOfField(assnTable.Columns, FieldOrder)].Formatted)
		if err != nil {
			return err
		}
		if orderInt > dayOrder {
			pk := row.Entries[api.GetPrimary(assnTable.Columns)].Formatted
			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				UpdateOnly: true,
				Table:      TableAssertions,
				Pk:         pk,
				Fields:     map[string]string{FieldOrder: fmt.Sprintf("%d", orderInt+1)},
			})
			if err != nil {
				return err
			}
			// NOTE We sync the row pks in reverse order so that we avoid a row overwriting its successor
			updated = append([]string{pk}, updated...)
		}
	}
	err = mv.syncPKs(TableAssertions, updated)
	if err != nil {
		return err
	}

	fields := map[string]string{
		FieldArg0:      dayPlanLink,
		FieldArg1:      fmt.Sprintf("[ ] %s", description),
		FieldARelation: ".Do Today",
		FieldOrder:     fmt.Sprintf("%d", dayOrder+1),
	}
	pk := fmt.Sprintf("%d", rand.Int63())
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      TableAssertions,
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
	newPlanView, err := g.SetView(NewPlanView, 4, 5, maxX-4, 9)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	newPlanView.Editable = true
	newPlanView.Editor = mv
	g.SetCurrentView(NewPlanView)
	newPlanView.Clear()
	newPlanView.Write([]byte("New Plan Description\n"))
	newPlanView.Write([]byte(mv.newPlanDescription))
	return nil
}

func (mv *MainView) queryForTaskLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	queryTasksView, err := g.SetView(QueryTasksView, 4, 5, maxX-4, maxY-7)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	queryTasksView.Highlight = true
	queryTasksView.SelBgColor = gocui.ColorWhite
	queryTasksView.SelFgColor = gocui.ColorBlack
	queryTasksView.Editable = true
	queryTasksView.Editor = mv
	queryTasksView.Clear()
	g.SetCurrentView(QueryTasksView)
	for _, task := range mv.filteredTasks {
		spaces := maxX - len(task)
		if spaces > 0 {
			task += strings.Repeat(" ", spaces)
		}
		queryTasksView.Write([]byte(task + "\n"))
	}
	query, err := g.SetView(QueryView, 4, maxY-7, maxX-4, maxY-5)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	query.Clear()
	query.Write([]byte(mv.topicQ))
	return nil
}

func (mv *MainView) listTasksLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	counts, err := g.SetView(CountsView, 0, 0, (maxX*3)/4, 2)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	counts.Clear()
	for _, span := range Spans {
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
		fmt.Fprintf(counts, "%s%s %s    ", prefix, Span2Title[span], suffix)
	}
	tasks, err := g.SetView(TasksView, 0, 3, (maxX*3)/4, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	tasks.Clear()
	g.SetCurrentView(TasksView)
	log, err := g.SetView(LogView, (maxX*3/4)+1, 0, maxX-1, maxY-1)
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

	logTable := mv.tables[TableLog]
	logDescriptionField := api.IndexOfField(logTable.Columns, FieldLogDescription)
	beginField := api.IndexOfField(logTable.Columns, FieldBegin)
	endField := api.IndexOfField(logTable.Columns, FieldEnd)

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
	if mv.span == Today {
		wasNil := mv.cachedTodayTasks == nil
		today, err := mv.todayTasks()
		if err != nil {
			return nil, err
		}
		mv.cachedTodayTasks = today
		if wasNil || mv.justSwitchedGrouping {
			mv.justSwitchedGrouping = false
			mv.selectNextFreeTask(g, v)
		}
		return mv.cachedTodayTasks, nil
	}
	tasksTable := mv.tables[TableTasks]
	projectField := api.IndexOfField(tasksTable.Columns, FieldPrimaryGoal)
	descriptionField := api.IndexOfField(tasksTable.Columns, FieldDescription)

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
	tasksTable := mv.tables[TableTasks]
	today := []DayItem{}
	for _, item := range mv.today {
		// Fall back to using the item's description as its attention
		// cycle if this is a one-off or we can't find its primary for some
		// reason
		brk := item.Description
		meta := mv.today2item[item.Description]
		taskPK := meta.TaskPK
		if taskPK == "" {
			taskPK = stripDayPlanPrefix(item.Description)
		}
		resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
			Table: TableTasks,
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

func (mv *MainView) itemStorePath() string {
	// NOTE this assumes only one timedb in the working dir from which this was invoked
	workdir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("Could not get working directory for item store path: %s", err))
	}
	return filepath.Join(workdir, ".execute.item_store.json")
}

func (mv *MainView) save() error {
	_, err := mv.dbms.Persist(ctx, &jqlpb.PersistRequest{})
	if err != nil {
		return err
	}
	// Persist the today2item mapping so that we can restore it later to use as
	// a base. Otherwise the mapping is hard to reconstruct since we only query
	// for active tasks when we construct it and some tasks might already be done.
	//
	// NOTE If this file gets too big I can just purge its entries every time
	// I create a new 'Plan today' task.
	itemStoreMarshaled, err := json.MarshalIndent(mv.today2item, "", "    ")
	if err != nil {
		return err
	}
	itemStore, err := os.OpenFile(mv.itemStorePath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer itemStore.Close()
	_, err = itemStore.Write(itemStoreMarshaled)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	err := g.SetKeybinding(TasksView, 'k', gocui.ModNone, mv.cursorUp)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'j', gocui.ModNone, mv.cursorDown)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(TasksView, gocui.KeyEnter, gocui.ModNone, mv.logTime); err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'w', gocui.ModNone, mv.openLink)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'i', gocui.ModNone, mv.bumpStatus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'I', gocui.ModNone, mv.degradeStatus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'l', gocui.ModNone, mv.nextSpan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'h', gocui.ModNone, mv.prevSpan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'a', gocui.ModNone, mv.switchModes)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'f', gocui.ModNone, mv.toggleFocus)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'X', gocui.ModNone, mv.refreshTasks)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'p', gocui.ModNone, mv.wrapTaskInRamps)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(NewPlansView, 'x', gocui.ModNone, mv.toggleAllPlans)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'x', gocui.ModNone, mv.taskMarker(StatusSatisfied))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'z', gocui.ModNone, mv.taskMarker(StatusFailed))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'Z', gocui.ModNone, mv.taskMarker(StatusAbandoned))
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'd', gocui.ModNone, mv.deleteDayPlan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 's', gocui.ModNone, mv.substituteTaskWithPrompt)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(QueryTasksView, gocui.KeyEnter, gocui.ModNone, mv.selectQueryItem); err != nil {
		return err
	}
	if err := g.SetKeybinding(NewPlanView, gocui.KeyEnter, gocui.ModNone, mv.createNewPlanFromInput); err != nil {
		return err
	}
	if err := g.SetKeybinding(NewPlansView, 'j', gocui.ModNone, mv.basicCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(NewPlansView, 'k', gocui.ModNone, mv.basicCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(NewPlansView, gocui.KeySpace, gocui.ModNone, mv.markPlanSelection); err != nil {
		return err
	}
	if err := g.SetKeybinding(NewPlansView, gocui.KeyEnter, gocui.ModNone, mv.substitutePlanSelections); err != nil {
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
	for ix, span := range Spans {
		ixs[span] = ix
	}
	mv.span = Spans[(ixs[mv.span]+1)%len(Spans)]
	if mv.span == Today {
		mv.selectNextFreeTask(g, v)
	} else {
		v.SetCursor(0, 0)
	}
	return mv.refreshView(g)
}

func (mv *MainView) prevSpan(g *gocui.Gui, v *gocui.View) error {
	ixs := map[string]int{}
	for ix, span := range Spans {
		ixs[span] = ix
	}
	prevIx := (ixs[mv.span] - 1)
	if prevIx == -1 {
		prevIx = len(Spans) - 1
	}
	mv.span = Spans[prevIx]
	if mv.span == Today {
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
	if ok {
		if meta, ok := mv.today2item[item.Description]; ok {
			currentPK = meta.TaskPK
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
	if mv.span == Today {
		return mv.insertNewPlan(g, v)
	}
	return mv.addToStatus(g, v, 1)
}

func (mv *MainView) degradeStatus(g *gocui.Gui, v *gocui.View) error {
	return mv.addToStatus(g, v, -1)
}

func (mv *MainView) addToStatus(g *gocui.Gui, v *gocui.View, delta int) error {
	// TODO getting selected task is very common. Should factor out.
	tasksTable := mv.tables[TableTasks]
	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[mv.span][oy+cy]
	pk := selectedTask.Entries[api.IndexOfField(tasksTable.Columns, FieldDescription)].Formatted

	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  TableTasks,
		Pk:     pk,
		Column: FieldStatus,
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

func (mv *MainView) openLink(g *gocui.Gui, v *gocui.View) error {
	pk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return err
	}
	tasksTable := mv.tables[TableTasks]
	nounsTable := mv.tables[TableNouns]
	task, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: TableTasks,
		Pk:    pk,
	})
	if err != nil {
		return err
	}
	direct := task.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldDirect)].Formatted
	obj, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: TableNouns,
		Pk:    direct,
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("txtopen", obj.Row.Entries[api.IndexOfField(nounsTable.Columns, FieldLink)].Formatted)
	return cmd.Run()
}

func (mv *MainView) logTime(g *gocui.Gui, v *gocui.View) error {
	tasksTable := mv.tables[TableTasks]
	logTable := mv.tables[TableLog]
	var cy, oy int
	view, err := g.View(TasksView)
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
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask.Entries[api.IndexOfField(tasksTable.Columns, FieldDescription)].Formatted), selectedTask, false)
		if err != nil {
			return err
		}
	} else if mv.log[0].Entries[api.IndexOfField(logTable.Columns, FieldEnd)].Formatted == "31 Dec 1969 16:00:00" {
		pk := mv.log[0].Entries[api.IndexOfField(logTable.Columns, FieldLogDescription)].Formatted
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      TableLog,
			Pk:         pk,
			Fields:     map[string]string{FieldEnd: ""},
		})
		if err != nil {
			return err
		}
	} else {
		pk := mv.log[0].Entries[api.IndexOfField(logTable.Columns, FieldLogDescription)].Formatted
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
	tasksTable := mv.tables[TableTasks]
	fields := map[string]string{
		FieldBegin: "",
		FieldTask:  selectedTask.Entries[api.IndexOfField(tasksTable.Columns, FieldDescription)].Formatted,
	}
	if andFinish {
		fields[FieldEnd] = ""
	}
	_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  TableLog,
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
	if mv.span == Today {
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
	tasksTable := mv.tables[TableTasks]
	return today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted, nil
}

func (mv *MainView) ResolveSelectedPK(g *gocui.Gui) (string, error) {
	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return "", err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}
	ix := oy + cy
	if mv.span == Today {
		item, ok := mv.ix2item[ix]
		if !ok {
			return stripDayPlanPrefix(item.Description), nil
		}
		meta, ok := mv.today2item[item.Description]
		if !ok {
			return stripDayPlanPrefix(item.Description), nil
		}
		return meta.TaskPK, nil
	} else {
		tasksTable := mv.tables[TableTasks]
		selectedTask := mv.tasks[mv.span][ix]
		return selectedTask.Entries[api.IndexOfField(tasksTable.Columns, FieldDescription)].Formatted, nil
	}
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	tasksTable := mv.tables[TableTasks]
	descriptionField := api.IndexOfField(tasksTable.Columns, FieldDescription)
	projectField := api.IndexOfField(tasksTable.Columns, FieldPrimaryGoal)
	spanField := api.IndexOfField(tasksTable.Columns, FieldSpan)
	statusField := api.IndexOfField(tasksTable.Columns, FieldStatus)

	active, err := mv.queryAllTasks(StatusPlanned, StatusActive)
	if err != nil {
		return err
	}
	mv.tasks = map[string]([]*jqlpb.Row){}
	for _, task := range active.Rows {
		span := task.Entries[spanField].Formatted
		// qurater scope tasks are good to keep an eye on, but to keep the
		// UX simple let's lump then in with the tasks for "this month"
		if span == SpanQuarter {
			span = SpanMonth
		}
		// If the task has already been started then mark it as active for today
		if task.Entries[statusField].Formatted == "Active" {
			span = SpanDay
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
		mv.tasks[SpanPending] = append(mv.tasks[SpanPending], task)
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
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	mv.log = []*jqlpb.Row{}
	if mv.span != Today {
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

func (mv *MainView) loadBaseToday2Item() (map[string]DayItemMeta, error) {
	// Restore the persisted today2item mapping to use as base. Otherwise the
	// mapping is hard to reconstruct since we only query
	// for active tasks when we construct it and some tasks might already be done.
	today2item := map[string]DayItemMeta{}
	contents, err := os.ReadFile(mv.itemStorePath())
	if err != nil {
		if os.IsNotExist(err) {
			return today2item, nil
		}
		return nil, err
	}
	err = json.Unmarshal(contents, &today2item)
	return today2item, err
}

func (mv *MainView) refreshToday() error {
	mv.today = []DayItem{}
	today2item, err := mv.loadBaseToday2Item()
	if err != nil {
		return err
	}
	mv.today2item = today2item

	today, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if today == nil {
		return nil
	}
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Do Today"}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	currentBreak := ""
	for _, row := range resp.Rows {
		val := row.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
		if !strings.HasPrefix(val, "[") {
			currentBreak = val
			continue
		}
		mv.today = append(mv.today, DayItem{
			Description: val,
			Break:       currentBreak,
			PK:          row.Entries[api.GetPrimary(assnTable.Columns)].Formatted,
		})
	}
	newTasks, err := mv.gatherNewTasks()
	if err != nil {
		return err
	}

	for _, newTask := range newTasks {
		mv.today2item[newTask.Description] = newTask
	}
	return nil
}

func (mv *MainView) queryInProgressTasks(ignore string) ([]string, error) {
	tasksTable := mv.tables[TableTasks]
	tasks, err := mv.queryAllTasks(StatusActive, StatusHabitual)
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
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldStatus,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: status}},
					},
				},
			},
		},
		OrderBy: FieldDescription,
	})
}

func (mv *MainView) queryLogs(task *jqlpb.Row) (*jqlpb.ListRowsResponse, error) {
	tasksTable := mv.tables[TableTasks]
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableLog,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldTask,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: task.Entries[api.IndexOfField(tasksTable.Columns, FieldDescription)].Formatted}},
					},
				},
			},
		},
		OrderBy: FieldBegin,
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
		if IsAttentionCycle(table, task) {
			return task, nil
		}
		seen[pk] = true
		parent := task.Entries[api.IndexOfField(table.Columns, FieldPrimaryGoal)].Formatted
		resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: TableTasks,
			Conditions: []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: FieldDescription,
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
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldStatus,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: []string{StatusActive, StatusHabitual}}},
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
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: taskCols}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Plan"}},
					},
				},
			},
		},
	})
}

func (mv *MainView) queryDayPlan() (*jqlpb.Row, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Plan"}},
					},
					{
						Column: FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "today"}},
					},
					{
						Column: FieldSpan,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Day"}},
					},
					{
						Column: FieldStatus,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Active"}},
					},
				},
			},
		},
		OrderBy: FieldStart,
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
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldAction,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Plan"}},
					},
					{
						Column: FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "today"}},
					},
					{
						Column: FieldSpan,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "Day"}},
					},
				},
			},
		},
		OrderBy: FieldStart,
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

func (mv *MainView) queryExistingTasks(planPK string) (map[string]bool, error) {
	assnTable := mv.tables[TableAssertions]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
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
		task := row.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
		if !strings.HasPrefix(task, "[ ] ") {
			continue
		}
		existing[task] = true
	}
	return existing, nil
}

func (mv *MainView) copyOldTasks() error {
	tasksTable := mv.tables[TableTasks]
	assnTable := mv.tables[TableAssertions]

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
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
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
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
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
		rel := oldBullet.Entries[api.IndexOfField(assnTable.Columns, FieldARelation)].Formatted
		val := oldBullet.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
		order := oldBullet.Entries[api.IndexOfField(assnTable.Columns, FieldOrder)].Formatted

		if isDayTaskDone(val) {
			continue
		}
		// pk doesn't really matter here so using a random integer
		pk := fmt.Sprintf("%d", rand.Int63())
		fields := map[string]string{
			FieldArg0:      fmt.Sprintf("tasks %s", todayPK),
			FieldArg1:      val,
			FieldARelation: rel,
			FieldOrder:     order,
		}
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:  TableAssertions,
			Pk:     pk,
			Fields: fields,
		})
		if err != nil {
			return err
		}
	}
	return mv.save()
}

func (mv *MainView) gatherNewTasks() ([]DayItemMeta, error) {
	// gather active and habitual tasks
	// gather each plan for those tasks
	// show an item if it is a plan or if it is an active leaf task with no plans
	// save contents
	tasksTable := mv.tables[TableTasks]
	assnTable := mv.tables[TableAssertions]
	tasks, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return nil, err
	}

	allTasks := []string{}
	task2children := map[string]([]*jqlpb.Row){}
	task2plans := map[string][]DayItemMeta{}

	for _, task := range tasks.Rows {
		allTasks = append(allTasks, task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)
		parent := task.Entries[api.IndexOfField(tasksTable.Columns, FieldPrimaryGoal)].Formatted
		task2children[parent] = append(task2children[parent], task)
	}

	plans, err := mv.queryPlans(allTasks)
	if err != nil {
		return nil, err
	}
	items := []DayItemMeta{}
	for _, plan := range plans.Rows {
		planString := plan.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
		// only include active plans though we query for all plans here because they may be useful later
		if strings.HasPrefix(planString, "[x] ") {
			continue
		}
		if !strings.HasPrefix(planString, "[ ] ") {
			planString = "[ ] " + planString
		}
		task := plan.Entries[api.IndexOfField(assnTable.Columns, FieldArg0)].Formatted[len("tasks "):]

		task2plans[task] = append(task2plans[task], DayItemMeta{
			Description: planString,
			TaskPK:      task,
			AssertionPK: plan.Entries[api.GetPrimary(assnTable.Columns)].Formatted,
		})
	}
	for _, task := range tasks.Rows {
		pk := task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
		status := task.Entries[api.IndexOfField(tasksTable.Columns, FieldStatus)].Formatted
		if status != "Active" || len(task2children[pk]) != 0 || len(task2plans[pk]) != 0 {
			continue
		}
		action := task.Entries[api.IndexOfField(tasksTable.Columns, FieldAction)].Formatted
		direct := task.Entries[api.IndexOfField(tasksTable.Columns, FieldDirect)].Formatted
		indirect := task.Entries[api.IndexOfField(tasksTable.Columns, FieldIndirect)].Formatted
		// no need for self reference here
		if action == "Plan" && direct == "today" && indirect == "" {
			continue
		}
		items = append(items, DayItemMeta{
			Description: fmt.Sprintf("[ ] %s", pk),
			TaskPK:      pk,
		})
	}
	for _, taskPlans := range task2plans {
		for _, item := range taskPlans {
			items = append(items, item)
		}
	}
	return items, nil
}

func (mv *MainView) insertNewTasks() error {
	tasksTable := mv.tables[TableTasks]

	newTasks, err := mv.gatherNewTasks()
	if err != nil {
		return err
	}
	dayPlan, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if dayPlan == nil {
		return nil
	}
	dayPlanPK := dayPlan.Entries[api.GetPrimary(tasksTable.Columns)].Formatted
	existingTasks, err := mv.queryExistingTasks(dayPlanPK)
	if err != nil {
		return err
	}

	for ix, item := range newTasks {
		if existingTasks[item.Description] {
			continue
		}
		// pk doesn't really matter here so using a random integer
		pk := fmt.Sprintf("%d", rand.Int63())
		fields := map[string]string{
			FieldArg0:      fmt.Sprintf("tasks %s", dayPlanPK),
			FieldArg1:      item.Description,
			FieldARelation: ".To Plan", // In a breakdown of Do Today, Do Tomorrow, & To Plan we add to the end
			FieldOrder:     fmt.Sprintf("%d", ix+len(existingTasks)),
		}
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:  TableAssertions,
			Pk:     pk,
			Fields: fields,
		})
		if err != nil {
			return err
		}
	}
	return mv.save()
}

func (mv *MainView) refreshTasks(g *gocui.Gui, v *gocui.View) error {
	// TODO(rabrams) this whole sequence is pretty inefficient. It involves multiple redundant
	// O(n) operations plus loading and re-loading the data.
	_, err := api.RunMacro(ctx, mv.dbms, "jql-timedb-autofill", api.MacroCurrentView{}, false)
	if err != nil {
		return err
	}
	err = mv.load(g)
	if err != nil {
		return err
	}
	err = mv.copyOldTasks()
	if err != nil {
		return err
	}
	err = mv.load(g)
	if err != nil {
		return err
	}
	err = mv.insertNewTasks()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) taskMarker(status string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return mv.markTask(g, v, status)
	}
}

func (mv *MainView) markTask(g *gocui.Gui, v *gocui.View, status string) error {
	if mv.span != Today {
		return nil
	}
	tasksView, err := g.View(TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()

	ix := oy + cy
	if ix >= len(mv.cachedTodayTasks) {
		return nil
	}
	// this is a bit of a hack since the today view can present tasks in different trees
	// so we ony want to mark the selection if it actually is a task and clear any prefixes
	selection := strings.TrimLeft(mv.cachedTodayTasks[ix], " ")
	if !strings.HasPrefix(selection, "[") {
		return nil
	}

	newVal := strings.Replace(selection, "[ ]", "[-]", 1)
	if status == StatusSatisfied {
		newVal = strings.Replace(selection, "[ ]", "[x]", 1)
	}
	// TODO(rabrams) this code predates having ix2item. See if it can be cleaned up with it.
	for _, item := range mv.today {
		if item.Description != selection {
			continue
		}
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      TableAssertions,
			Pk:         item.PK,
			Fields:     map[string]string{FieldArg1: newVal},
		})
		if err != nil {
			return err
		}
	}

	meta, ok := mv.today2item[selection]
	if !ok {
		// Likely a one-off task in our plan so has no source to update
		return nil
	}

	mv.today2item[newVal] = meta // re-map the today-plan to its item so it can still map back to its task PK
	if meta.AssertionPK != "" {
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      TableAssertions,
			Pk:         meta.AssertionPK,
			Fields:     map[string]string{FieldArg1: newVal},
		})
		if err != nil {
			return err
		}
	} else if meta.TaskPK != "" {
		_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			UpdateOnly: true,
			Table:      TableTasks,
			Pk:         meta.TaskPK,
			Fields:     map[string]string{FieldStatus: status},
		})
		if err != nil {
			return err
		}
	}
	// Sadly no support for unmarking a task because by this point we've lost the context
	// on where the task came from. You have to manually unmark it.
	err = mv.save()
	if err != nil {
		return err
	}
	err = mv.cursorDown(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) deleteDayPlan(g *gocui.Gui, v *gocui.View) error {
	if mv.span != Today {
		return nil
	}
	tasksView, err := g.View(TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	_, err = mv.dbms.DeleteRow(ctx, &jqlpb.DeleteRowRequest{
		Table: TableAssertions,
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
	Domain     string
	Direct     string
	TaskPK     string
	IsPrepTask bool
	IsWarmup   bool
}

func (mv *MainView) GetCurrentDomain(g *gocui.Gui, v *gocui.View) (CurrentDomainInfo, error) {
	tasksTable := mv.tables[TableTasks]
	taskPk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return CurrentDomainInfo{}, err
	}
	resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: TableTasks,
		Pk:    taskPk,
	})
	if err != nil {
		return CurrentDomainInfo{}, err
	}
	direct := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldDirect)].Formatted
	indirect := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldIndirect)].Formatted
	isPrepareTask := (direct == "" &&  indirect == "")
	isWarmup := resp.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldAction)].Formatted == "Warm-up"
	cycle, err := mv.retrieveAttentionCycle(tasksTable, resp.Row)
	if err != nil {
		return CurrentDomainInfo{}, err
	}
	domain := cycle.Entries[api.IndexOfField(tasksTable.Columns, FieldIndirect)].Formatted
	return CurrentDomainInfo{
		IsPrepTask: isPrepareTask,
		Direct:     direct,
		Domain:     domain,
		TaskPK:     taskPk,
		IsWarmup:   isWarmup,
	}, nil
}

func (mv *MainView) InjectTaskWithAllMatching(g *gocui.Gui, v *gocui.View) (int, error) {
	// Return the count of added items so that a higher level caller can decide to redirect
	// the user to populate new items or not
	tasksTable := mv.tables[TableTasks]
	taskPk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return 0, err
	}
	resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: TableTasks,
		Pk:    taskPk,
	})
	if err != nil {
		return 0, err
	}
	cycle, err := mv.retrieveAttentionCycle(tasksTable, resp.Row)
	if err != nil {
		return 0, err
	}
	cycleName := cycle.Entries[api.GetPrimary(mv.tables[TableTasks].Columns)].Formatted
	activeDescendants, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldPrimaryGoal,
						Match: &jqlpb.Filter_PathToMatch{&jqlpb.PathToMatch{Value: cycleName}},
					},
					{
						Column: FieldStatus,
						Match: &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: StatusActive}},
					},
				},
			},
		},
	})
	if err != nil {
		return 0, err
	}
	descPKs := []string{}
	for _, row := range activeDescendants.Rows {
		pk := row.Entries[api.GetPrimary(mv.tables[TableTasks].Columns)].Formatted
		descPKs = append(descPKs, pk)
	}
	alreadyPresent, err := mv.queryPresentAndFutureDayPlanNames()
	if err != nil {
		return 0, nil
	}
	added := 0
	for _, descPK := range descPKs {
		if alreadyPresent[descPK] {
			continue
		}
		err := mv.insertDayPlan(g, descPK, 0)
		if err != nil {
			return added, err
		}
		added += 1
	}
	return added, mv.refreshView(g)
}

func (mv *MainView) queryPresentAndFutureDayPlanNames() (map[string]bool, error) {
	today, err := mv.queryDayPlan()
	if err != nil {
		return nil, err
	}
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", today.Entries[api.GetPrimary(tasksTable.Columns)].Formatted)}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, row := range resp.Rows {
		arg1 := row.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
		if !isAssertionDayPlan(arg1) {
			continue
		}
		names[stripDayPlanPrefix(arg1)] = true
	}
	return names, nil
}

func (mv *MainView) substituteTaskWithPrompt(g *gocui.Gui, v *gocui.View) error {
	if mv.span != Today {
		return nil
	}
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	meta := mv.today2item[item.Description]
	isTask := meta.AssertionPK == ""
	if isTask {
		return mv.substituteTaskWithPlans(g, meta.TaskPK)
	} else {
		// TODO(rabrams) bit of a hack to strip the plan of its prefix
		return mv.substitutePlanWithImplementation(g, meta.Description[len("[ ] "):])
	}
}

func (mv *MainView) substituteTaskWithPlans(g *gocui.Gui, taskPK string) error {
	mv.substitutingPlan = false
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	task, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: TableTasks,
		Pk:    taskPK,
	})
	if err != nil {
		return err
	}
	direct := task.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldDirect)].Formatted
	action := task.Row.Entries[api.IndexOfField(tasksTable.Columns, FieldAction)].Formatted
	procedures, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: "nouns " + direct}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Procedure"}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return err
	}
	// TODO this probably has a lot in common with logic in the procedure navigator
	// so should be made into a shared library
	procedure := ""
	prefix := fmt.Sprintf("### %s\n", action)
	for _, proc := range procedures.Rows {
		procText := proc.Entries[api.IndexOfField(assnTable.Columns, FieldArg1)].Formatted
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
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	candidates, err := mv.queryAllTasks(StatusActive, StatusHabitual, StatusPlanned, StatusPending)
	if err != nil {
		return err
	}
	candidatePKs := map[string]bool{}
	for _, candidate := range candidates.Rows {
		candidatePKs[candidate.Entries[api.GetPrimary(tasksTable.Columns)].Formatted] = true
	}
	implementations, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg1,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: plan}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Implements"}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return err
	}
	items := []PlanSelectionItem{}
	for _, row := range implementations.Rows {
		pk := row.Entries[api.IndexOfField(assnTable.Columns, FieldArg0)].Formatted[len("tasks "):]
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
	newPlansView, err := g.SetView(NewPlansView, 4, 5, maxX-4, len(mv.planSelections)+8)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	newPlansView.Editable = true
	newPlansView.Highlight = true
	newPlansView.SelBgColor = gocui.ColorWhite
	newPlansView.SelFgColor = gocui.ColorBlack
	newPlansView.Editor = mv
	g.SetCurrentView(NewPlansView)
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

func (mv *MainView) wrapTaskInRamps(g *gocui.Gui, v *gocui.View) error {
	if mv.span != Today || mv.MainViewMode != MainViewModeListBar {
		return nil
	}
	pk, err := mv.ResolveSelectedPK(g)
	if err != nil {
		return err
	}
	for _, action := range []string{"Prepare", "Wrap-up"} {
		fields := map[string]string{
			FieldAction:      action,
			FieldPrimaryGoal: pk,
			FieldStart:       "",
			FieldStatus:      StatusActive,
		}
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table:  TableTasks,
			Pk:     "",
			Fields: fields,
		})
		if err != nil {
			return err
		}
		view := api.MacroCurrentView{
			Table:            TableTasks,
			PrimarySelection: "",
		}
		_, err = api.RunMacro(ctx, mv.dbms, "jql-timedb-setpk --v2", view, true)
		if err != nil {
			return err
		}
	}
	created, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableTasks,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldPrimaryGoal,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: pk}},
					},
					{
						Column: FieldDirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
					},
					{
						Column: FieldIndirect,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
					},
				},
			},
		},
		OrderBy: FieldAction,
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
	err := g.DeleteView(NewPlansView)
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
			Table: TableTasks,
			Pk:    taskPK,
			Fields: map[string]string{
				FieldSpan:   "Day",
				FieldStart:  "",
				FieldStatus: "Active",
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
	err = mv.markTask(g, v, StatusSatisfied)
	if err != nil {
		return err
	}
	err = mv.syncPKs(TableTasks, updated)
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
	tasksView, err := g.View(TasksView)
	if err != nil {
		return err
	}
	_, oy := tasksView.Origin()
	_, cy := tasksView.Cursor()
	ix := oy + cy
	item := mv.ix2item[ix]
	meta := mv.today2item[item.Description]
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
	err = g.DeleteView(NewPlansView)
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
	assnTable := mv.tables[TableAssertions]
	tasksTable := mv.tables[TableTasks]
	pending, err := mv.queryAllTasks(StatusPending)
	if err != nil {
		return nil, err
	}
	pk2task := map[string](*jqlpb.Row){}
	for _, task := range pending.Rows {
		pk2task[task.Entries[api.GetPrimary(tasksTable.Columns)].Formatted] = task
	}
	implementations, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Implements"}},
					},
				},
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return nil, err
	}
	for _, assn := range implementations.Rows {
		obj := assn.Entries[api.IndexOfField(assnTable.Columns, FieldArg0)]
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

func stripDayPlanPrefix(s string) string {
	return s[len("[ ] "):]
}
