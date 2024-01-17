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
	"syscall"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/types"
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
	OSM  *osm.ObjectStoreMapper
	dbms api.JQL_DBMS

	MainViewMode MainViewMode
	TaskViewMode TaskViewMode

	// maps span to tasks of that span
	tasks map[string]([]*jqlpb.Row)
	span  string
	log   []*jqlpb.Row
	path  string

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
func NewMainView(path string, g *gocui.Gui, dbms api.JQL_DBMS, mapper *osm.ObjectStoreMapper) (*MainView, error) {
	rand.Seed(time.Now().UnixNano())
	mv := &MainView{
		OSM:  mapper,
		dbms: dbms,
		path: path,
	}
	return mv, mv.load(g)
}

func (mv *MainView) load(g *gocui.Gui) error {
	err := mv.OSM.Load()
	if err != nil {
		return err
	}
	mv.MainViewMode = MainViewModeListBar
	mv.tasks = map[string]([]*jqlpb.Row){}
	mv.span = Today
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
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	newOrder := 0
	plansResp, err := mv.queryPlans([]string{taskPK})
	if err != nil {
		return err
	}
	for _, plan := range plansResp.Rows {
		orderInt, err := strconv.Atoi(plan.Entries[assnTable.IndexOfField(FieldOrder)].Formatted)
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
	err = assnTable.InsertWithFields(pk, fields)
	if err != nil {
		return err
	}
	err = mv.insertDayPlan(g, description)
	if err != nil {
		return err
	}
	err = mv.save()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) insertDayPlan(g *gocui.Gui, description string) error {
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
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
	dayPlanLink := fmt.Sprintf("tasks %s", dayPlan.Entries[tasksTable.Primary()].Formatted)
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
	})
	if err != nil {
		return err
	}
	dayOrder := 0
	for _, row := range existingTodos.Rows {
		if row.Entries[assnTable.IndexOfField(FieldArg1)].Formatted == insertsAfter.Description {
			dayOrder, err = strconv.Atoi(row.Entries[assnTable.IndexOfField(FieldOrder)].Formatted)
			if err != nil {
				return err
			}
		}
	}
	for _, row := range existingTodos.Rows {
		orderInt, err := strconv.Atoi(row.Entries[assnTable.IndexOfField(FieldOrder)].Formatted)
		if err != nil {
			continue
		}
		if orderInt > dayOrder {
			err = assnTable.Update(row.Entries[assnTable.Primary()].Formatted, FieldOrder, fmt.Sprintf("%d", orderInt+1))
			if err != nil {
				return err
			}
		}
	}
	fields := map[string]string{
		FieldArg0:      dayPlanLink,
		FieldArg1:      fmt.Sprintf("[ ] %s", description),
		FieldARelation: ".Do Today",
		FieldOrder:     fmt.Sprintf("%d", dayOrder+1),
	}
	pk := fmt.Sprintf("%d", rand.Int63())
	err = assnTable.InsertWithFields(pk, fields)
	if err != nil {
		return err
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

	logTable := mv.OSM.GetDB().Tables[TableLog]

	logDescriptionField := logTable.IndexOfField(FieldLogDescription)
	beginField := logTable.IndexOfField(FieldBegin)
	endField := logTable.IndexOfField(FieldEnd)

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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	projectField := taskTable.IndexOfField(FieldPrimaryGoal)
	descriptionField := taskTable.IndexOfField(FieldDescription)

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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	today := []DayItem{}
	for _, item := range mv.today {
		// Fall back to using the item's description as its attention
		// cycle if this is a one-off or we can't find its primary for some
		// reason
		brk := item.Description
		meta, ok := mv.today2item[item.Description]
		resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
			Table: TableTasks,
			Pk:    meta.TaskPK,
		})
		if err != nil {
			return nil, err
		}
		if ok {
			task, err := mv.retrieveAttentionCycle(taskTable, resp.Row)
			if err == nil {
				brk = task.Entries[taskTable.Primary()].Formatted
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
		if strings.HasPrefix(item.Description, "[x]") {
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
	return filepath.Join(filepath.Dir(mv.path), ".item_store."+filepath.Base(mv.path))
}

func (mv *MainView) save() error {
	f, err := os.OpenFile(mv.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	err = mv.OSM.StoreEntries()
	if err != nil {
		return err
	}
	// Persist the today2item mapping so that we can restore it later to use as
	// a base. Otherwise the mapping is hard to reconstruct since we only query
	// for active tasks when we construct it and some tasks might already be done.
	//
	// NOTE If this file gets too big I can just purge its entries every time
	// I create a new 'Plan today' task.
	itemStoreMarshaled, err := json.Marshal(mv.today2item)
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
	err = g.SetKeybinding(TasksView, 's', gocui.ModNone, mv.saveContents)
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
	err = g.SetKeybinding(TasksView, 'G', gocui.ModNone, mv.selectAndGoToTask)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'g', gocui.ModNone, mv.goToJQLEntry)
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
	err = g.SetKeybinding(NewPlansView, 'x', gocui.ModNone, mv.toggleAllPlans)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'x', gocui.ModNone, mv.markTask)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 't', gocui.ModNone, mv.goToToday)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'd', gocui.ModNone, mv.deleteDayPlan)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'S', gocui.ModNone, mv.substituteTask)
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

func (mv *MainView) selectAndGoToTask(g *gocui.Gui, v *gocui.View) error {
	err := mv.setTaskList(g, v)
	if err != nil {
		return err
	}
	return mv.queryForTask(g, v, func(taskPK string) error {
		return mv.goToPK(g, v, taskPK)
	})
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[mv.span][oy+cy]
	pk := selectedTask.Entries[taskTable.IndexOfField(FieldDescription)].Formatted

	new, err := taskTable.Entries[pk][taskTable.IndexOfField(FieldStatus)].Add(delta)
	if err != nil {
		return err
	}
	taskTable.Entries[pk][taskTable.IndexOfField(FieldStatus)] = new
	err = mv.saveContents(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) openLink(g *gocui.Gui, v *gocui.View) error {
	pk, err := mv.resolveSelectedPK(g)
	if err != nil {
		return err
	}
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	nounTable := mv.OSM.GetDB().Tables[TableNouns]
	task, ok := taskTable.Entries[pk]
	if !ok {
		return fmt.Errorf("Could not find selected pk: %s", pk)
	}
	direct := task[taskTable.IndexOfField(FieldDirect)].Format("")
	obj, ok := nounTable.Entries[direct]
	if !ok {
		return fmt.Errorf("Could not find direct object: %s", direct)
	}
	cmd := exec.Command("txtopen", obj[nounTable.IndexOfField(FieldLink)].Format(""))
	return cmd.Run()
}

func (mv *MainView) logTime(g *gocui.Gui, v *gocui.View) error {
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	logTable := mv.OSM.GetDB().Tables[TableLog]
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
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask.Entries[taskTable.IndexOfField(FieldDescription)].Formatted), selectedTask, false)
		if err != nil {
			return err
		}
	} else if mv.log[0].Entries[logTable.IndexOfField(FieldEnd)].Formatted == "31 Dec 1969 16:00:00" {
		pk := mv.log[0].Entries[logTable.IndexOfField(FieldLogDescription)].Formatted
		err = logTable.Update(pk, FieldEnd, "")
		if err != nil {
			return err
		}
	} else {
		pk := mv.log[0].Entries[logTable.IndexOfField(FieldLogDescription)].Formatted
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	logTable := mv.OSM.GetDB().Tables[TableLog]
	err := logTable.Insert(pk)
	if err != nil {
		return err
	}
	err = logTable.Update(pk, FieldBegin, "")
	if err != nil {
		return err
	}
	if andFinish {
		err = logTable.Update(pk, FieldEnd, "")
		if err != nil {
			return err
		}
	}
	return logTable.Update(pk, FieldTask, selectedTask.Entries[taskTable.IndexOfField(FieldDescription)].Formatted)
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

func (mv *MainView) goToToday(g *gocui.Gui, v *gocui.View) error {
	today, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if today == nil {
		return nil
	}
	return mv.goToPK(g, v, today.Entries[mv.OSM.GetDB().Tables[TableTasks].Primary()].Formatted)
}

func (mv *MainView) goToJQLEntry(g *gocui.Gui, v *gocui.View) error {
	pk, err := mv.resolveSelectedPK(g)
	if err != nil {
		return err
	}
	return mv.goToPK(g, v, pk)
}

func (mv *MainView) resolveSelectedPK(g *gocui.Gui) (string, error) {
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
			return "", nil
		}
		meta, ok := mv.today2item[item.Description]
		if !ok {
			return "", nil
		}
		return meta.TaskPK, nil
	} else {
		taskTable := mv.OSM.GetDB().Tables[TableTasks]
		selectedTask := mv.tasks[mv.span][ix]
		return selectedTask.Entries[taskTable.IndexOfField(FieldDescription)].Formatted, nil
	}
}

func (mv *MainView) goToPK(g *gocui.Gui, v *gocui.View, pk string) error {
	err := mv.saveContents(g, v)
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(JQLName)
	if err != nil {
		return err
	}

	args := []string{JQLName, "--path", mv.path, "--table", TableTasks, "--pk", pk}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	descriptionField := taskTable.IndexOfField(FieldDescription)
	projectField := taskTable.IndexOfField(FieldPrimaryGoal)
	spanField := taskTable.IndexOfField(FieldSpan)
	statusField := taskTable.IndexOfField(FieldStatus)

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
			task, err = mv.retrieveAttentionCycle(taskTable, task)
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
			task, err = mv.retrieveAttentionCycle(taskTable, task)
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
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldArg0,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: fmt.Sprintf("tasks %s", today.Entries[tasksTable.Primary()].Formatted)}},
					},
					{
						Column: FieldARelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Do Today"}},
					},
				},
			},
		},
	})
	currentBreak := ""
	for _, row := range resp.Rows {
		val := row.Entries[assertionsTable.IndexOfField(FieldArg1)].Formatted
		if !strings.HasPrefix(val, "[") {
			currentBreak = val
			continue
		}
		mv.today = append(mv.today, DayItem{
			Description: val,
			Break:       currentBreak,
			PK:          row.Entries[assertionsTable.Primary()].Formatted,
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	tasks, err := mv.queryAllTasks(StatusActive, StatusHabitual)
	if err != nil {
		return nil, err
	}
	pks := []string{}
	for _, task := range tasks.Rows {
		pk := task.Entries[taskTable.Primary()].Formatted
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	return mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableLog,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldTask,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: task.Entries[taskTable.IndexOfField(FieldDescription)].Formatted}},
					},
				},
			},
		},
		OrderBy: FieldBegin,
		Dec:     true,
	})
}

func (mv *MainView) retrieveAttentionCycle(table *types.Table, task *jqlpb.Row) (*jqlpb.Row, error) {
	orig := task
	seen := map[string]bool{}
	for {
		pk := task.Entries[table.Primary()].Formatted
		if seen[pk] {
			// hit a cycle
			return orig, nil
		}
		if IsAttentionCycle(table, task) {
			return task, nil
		}
		seen[pk] = true
		parent := task.Entries[table.IndexOfField(FieldPrimaryGoal)].Formatted
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
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]
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
		task := row.Entries[assertionsTable.IndexOfField(FieldArg1)].Formatted
		if !strings.HasPrefix(task, "[ ] ") {
			continue
		}
		existing[task] = true
	}
	return existing, nil
}

func (mv *MainView) copyOldTasks() error {
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]

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

	todayPK := today.Entries[taskTable.Primary()].Formatted
	yesterdayPK := yesterday.Entries[taskTable.Primary()].Formatted

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
		rel := oldBullet.Entries[assertionsTable.IndexOfField(FieldARelation)].Formatted
		val := oldBullet.Entries[assertionsTable.IndexOfField(FieldArg1)].Formatted
		order := oldBullet.Entries[assertionsTable.IndexOfField(FieldOrder)].Formatted

		if strings.HasPrefix(val, "[x] ") {
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
		err := assertionsTable.InsertWithFields(pk, fields)
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasks, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return nil, err
	}

	allTasks := []string{}
	task2children := map[string]([]*jqlpb.Row){}
	task2plans := map[string][]DayItemMeta{}

	for _, task := range tasks.Rows {
		allTasks = append(allTasks, task.Entries[taskTable.Primary()].Formatted)
		parent := task.Entries[taskTable.IndexOfField(FieldPrimaryGoal)].Formatted
		task2children[parent] = append(task2children[parent], task)
	}

	plans, err := mv.queryPlans(allTasks)
	if err != nil {
		return nil, err
	}
	items := []DayItemMeta{}
	for _, plan := range plans.Rows {
		planString := plan.Entries[assertionsTable.IndexOfField(FieldArg1)].Formatted
		// only include active plans though we query for all plans here because they may be useful later
		if strings.HasPrefix(planString, "[x] ") {
			continue
		}
		if !strings.HasPrefix(planString, "[ ] ") {
			planString = "[ ] " + planString
		}
		task := plan.Entries[assertionsTable.IndexOfField(FieldArg0)].Formatted[len("tasks "):]

		task2plans[task] = append(task2plans[task], DayItemMeta{
			Description: planString,
			TaskPK:      task,
			AssertionPK: plan.Entries[assertionsTable.Primary()].Formatted,
		})
	}
	for _, task := range tasks.Rows {
		pk := task.Entries[taskTable.Primary()].Formatted
		status := task.Entries[taskTable.IndexOfField(FieldStatus)].Formatted
		if status != "Active" || len(task2children[pk]) != 0 || len(task2plans[pk]) != 0 {
			continue
		}
		action := task.Entries[taskTable.IndexOfField(FieldAction)].Formatted
		direct := task.Entries[taskTable.IndexOfField(FieldDirect)].Formatted
		indirect := task.Entries[taskTable.IndexOfField(FieldIndirect)].Formatted
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
	taskTable := mv.OSM.GetDB().Tables[TableTasks]
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]

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
	dayPlanPK := dayPlan.Entries[taskTable.Primary()].Formatted
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
		err := assertionsTable.InsertWithFields(pk, fields)
		if err != nil {
			return err
		}
	}
	return mv.save()
}

func (mv *MainView) refreshPKs(g *gocui.Gui) error {
	err := exec.Command("jql-timedb-set-all-pks").Run()
	if err != nil {
		return err
	}
	err = mv.load(g)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) refreshTasks(g *gocui.Gui, v *gocui.View) error {
	// TODO(rabrams) this whole sequence is pretty inefficient. It involves multiple redundant
	// O(n) operations plus loading and re-loading the data.
	err := exec.Command("jql-timedb-autofill").Run()
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

func (mv *MainView) markTask(g *gocui.Gui, v *gocui.View) error {
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

	newVal := strings.Replace(selection, "[ ]", "[x]", 1)
	assertionsTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	// TODO(rabrams) this code predates having ix2item. See if it can be cleaned up with it.
	for _, item := range mv.today {
		if item.Description != selection {
			continue
		}
		err := assertionsTable.Update(item.PK, FieldArg1, newVal)
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
		err := assertionsTable.Update(meta.AssertionPK, FieldArg1, newVal)
		if err != nil {
			return err
		}
	} else if meta.TaskPK != "" {
		err := tasksTable.Update(meta.TaskPK, FieldStatus, StatusSatisfied)
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
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	err = assnTable.Delete(item.PK)
	if err != nil {
		return err
	}
	err = mv.save()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) substituteTask(g *gocui.Gui, v *gocui.View) error {
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
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	direct := tasksTable.Entries[taskPK][tasksTable.IndexOfField(FieldDirect)].Format("")
	action := tasksTable.Entries[taskPK][tasksTable.IndexOfField(FieldAction)].Format("")
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
		procText := proc.Entries[assnTable.IndexOfField(FieldArg1)].Formatted
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
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	candidates, err := mv.queryAllTasks(StatusActive, StatusHabitual, StatusPlanned, StatusPending)
	if err != nil {
		return err
	}
	candidatePKs := map[string]bool{}
	for _, candidate := range candidates.Rows {
		candidatePKs[candidate.Entries[tasksTable.Primary()].Formatted] = true
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
		pk := row.Entries[assnTable.IndexOfField(FieldArg0)].Formatted[len("tasks "):]
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
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	for _, item := range mv.planSelections {
		if !item.Marked {
			continue
		}
		inserted = true
		taskPK := item.Plan
		err = tasksTable.Update(taskPK, FieldSpan, "Day")
		if err != nil {
			return err
		}
		err = tasksTable.Update(taskPK, FieldStart, "")
		if err != nil {
			return err
		}
		err = tasksTable.Update(taskPK, FieldStatus, "Active")
		if err != nil {
			return err
		}
		mv.insertDayPlan(g, item.Plan)
	}
	// If the user didn't mark any selections then don't actually change anything
	if !inserted {
		return nil
	}
	// NOTE we rely on markTask to also save our changes
	err = mv.markTask(g, v)
	if err != nil {
		return err
	}
	err = mv.refreshPKs(g)
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
	assnTable := mv.OSM.GetDB().Tables[TableAssertions]
	tasksTable := mv.OSM.GetDB().Tables[TableTasks]
	pending, err := mv.queryAllTasks(StatusPending)
	if err != nil {
		return nil, err
	}
	pk2task := map[string](*jqlpb.Row){}
	for _, task := range pending.Rows {
		pk2task[task.Entries[tasksTable.Primary()].Formatted] = task
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
		obj := assn.Entries[assnTable.IndexOfField(FieldArg0)]
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
