package ui

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/img/jql/ui"
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int

const (
	MainViewModeListBar MainViewMode = iota
	MainViewModeListCycles
	MainViewModeSwitchingToJQL
	MainViewModeGoingToJQLEntry
)

// A MainView is the overall view including a project list
// and a detailed view of the current project
type MainView struct {
	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	Mode MainViewMode

	// maps span to tasks of that span
	tasks map[string]([][]types.Entry)
	span  string
	log   [][]types.Entry
	path  string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(path string, g *gocui.Gui) (*MainView, error) {
	rand.Seed(time.Now().UnixNano())
	mv := &MainView{
		path: path,
	}
	return mv, mv.load(g)
}

func (mv *MainView) load(g *gocui.Gui) error {
	var store storage.Store
	if strings.HasSuffix(mv.path, ".json") {
		store = &storage.JSONStore{}
	} else {
		return fmt.Errorf("unknown file type")
	}
	mapper, err := osm.NewObjectStoreMapper(store)
	if err != nil {
		return err
	}
	f, err := os.Open(mv.path)
	if err != nil {
		return err
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		return err
	}
	mv.OSM = mapper
	mv.DB = db
	mv.Mode = MainViewModeListBar
	mv.tasks = map[string]([][]types.Entry){}
	mv.span = SpanDay
	return mv.refreshView(g)
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	counts, err := g.SetView(CountsView, 0, 0, (maxX*3)/4, 2)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	counts.Clear()
	for _, span := range Spans {
		prefix := " "
		if span == mv.span {
			prefix = "*"
		}
		suffix := fmt.Sprintf("(%d)", len(mv.tasks[span]))
		if len(mv.tasks[span]) == 0 {
			suffix = ""
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
	tasks.Highlight = mv.Mode != MainViewModeSwitchingToJQL && mv.Mode != MainViewModeGoingToJQLEntry

	if mv.Mode == MainViewModeSwitchingToJQL || mv.Mode == MainViewModeGoingToJQLEntry {
		// HACK give the event loop time to clear highlighting from the tty
		// so that when we switch to jql we don't have inverted colors
		go func() {
			time.Sleep(50 * time.Millisecond)
			if mv.Mode == MainViewModeSwitchingToJQL {
				mv.switchToJQL(g, tasks)
			} else if mv.Mode == MainViewModeGoingToJQLEntry {
				mv.goToJQLEntry(g, tasks)
			}
		}()
	}

	for _, desc := range mv.tabulatedTasks() {
		fmt.Fprintf(tasks, "%s\n", desc)
	}

	logTable := mv.DB.Tables[TableLog]

	logDescriptionField := logTable.IndexOfField(FieldLogDescription)
	beginField := logTable.IndexOfField(FieldBegin)
	endField := logTable.IndexOfField(FieldEnd)

	for _, logEntry := range mv.log {
		fmt.Fprintf(
			log, "%s\n    %s - %s\n\n",
			logEntry[logDescriptionField].Format(""),
			logEntry[beginField].Format(""),
			logEntry[endField].Format(""),
		)
	}

	return nil
}

func (mv *MainView) tabulatedTasks() []string {
	taskTable := mv.DB.Tables[TableTasks]
	projectField := taskTable.IndexOfField(FieldPrimaryGoal)
	descriptionField := taskTable.IndexOfField(FieldDescription)

	// 10 char buffer
	buffer := 10
	maxChars := buffer
	for _, task := range mv.tasks[mv.span] {
		taskChars := len(task[descriptionField].Format("")) + buffer
		if taskChars > maxChars {
			maxChars = taskChars
		}
	}

	toret := []string{}

	for _, task := range mv.tasks[mv.span] {
		taskBuffer := maxChars - len(task[descriptionField].Format(""))
		toret = append(toret,
			fmt.Sprintf(" %s%s%s",
				task[descriptionField].Format(""),
				strings.Repeat(" ", taskBuffer),
				task[projectField].Format(""),
			))
	}
	return toret
}

func (mv *MainView) saveContents(g *gocui.Gui, v *gocui.View) error {
	return mv.save()
}

func (mv *MainView) save() error {
	f, err := os.OpenFile(mv.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	err = mv.OSM.Dump(mv.DB, f)
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
	err = g.SetKeybinding(TasksView, 'q', gocui.ModNone, mv.triggerSwitchToJQL)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'g', gocui.ModNone, mv.triggerGoToJQLEntry)
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
	err = g.SetKeybinding(TasksView, 'X', gocui.ModNone, mv.refreshTasks)
	if err != nil {
		return err
	}

	return nil
}

func (mv *MainView) nextSpan(g *gocui.Gui, v *gocui.View) error {
	ixs := map[string]int{}
	for ix, span := range Spans {
		ixs[span] = ix
	}
	mv.span = Spans[(ixs[mv.span]+1)%len(Spans)]
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
	return mv.refreshView(g)
}

func (mv *MainView) bumpStatus(g *gocui.Gui, v *gocui.View) error {
	return mv.addToStatus(g, v, 1)
}

func (mv *MainView) degradeStatus(g *gocui.Gui, v *gocui.View) error {
	return mv.addToStatus(g, v, -1)
}

func (mv *MainView) addToStatus(g *gocui.Gui, v *gocui.View, delta int) error {
	// TODO getting selected task is very common. Should factor out.
	taskTable := mv.DB.Tables[TableTasks]
	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[mv.span][oy+cy]
	pk := selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")

	new, err := selectedTask[taskTable.IndexOfField(FieldStatus)].Add(delta)
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
	taskTable := mv.DB.Tables[TableTasks]
	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedItem := mv.tasks[mv.span][oy+cy]
	cmd := exec.Command("txtopen", selectedItem[taskTable.IndexOfField(FieldLink)].Format(""))
	return cmd.Run()
}

func (mv *MainView) logTime(g *gocui.Gui, v *gocui.View) error {
	taskTable := mv.DB.Tables[TableTasks]
	logTable := mv.DB.Tables[TableLog]
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
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")), selectedTask, false)
		if err != nil {
			return err
		}
	} else if mv.log[0][logTable.IndexOfField(FieldEnd)].Format("") == "31 Dec 1969 16:00:00" {
		pk := mv.log[0][logTable.IndexOfField(FieldLogDescription)].Format("")
		err = logTable.Update(pk, FieldEnd, "")
		if err != nil {
			return err
		}
	} else {
		pk := mv.log[0][logTable.IndexOfField(FieldLogDescription)].Format("")
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

func (mv *MainView) newTime(g *gocui.Gui, pk string, selectedTask []types.Entry, andFinish bool) error {
	taskTable := mv.DB.Tables[TableTasks]
	logTable := mv.DB.Tables[TableLog]
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
	return logTable.Update(pk, FieldTask, selectedTask[taskTable.IndexOfField(FieldDescription)].Format(""))
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy+1); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+1); err != nil {
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

func (mv *MainView) triggerSwitchToJQL(g *gocui.Gui, v *gocui.View) error {
	mv.Mode = MainViewModeSwitchingToJQL
	return nil
}

func (mv *MainView) switchToJQL(g *gocui.Gui, v *gocui.View) error {
	err := mv.saveContents(g, v)
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(JQLName)
	if err != nil {
		return err
	}

	args := []string{JQLName, mv.path, TableTasks}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) triggerGoToJQLEntry(g *gocui.Gui, v *gocui.View) error {
	mv.Mode = MainViewModeGoingToJQLEntry
	return nil
}

func (mv *MainView) goToJQLEntry(g *gocui.Gui, v *gocui.View) error {
	taskTable := mv.DB.Tables[TableTasks]
	_, oy := v.Origin()
	_, cy := v.Cursor()
	selectedTask := mv.tasks[mv.span][oy+cy]
	pk := selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")

	err := mv.saveContents(g, v)
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(JQLName)
	if err != nil {
		return err
	}

	args := []string{JQLName, mv.path, TableTasks, pk}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	taskTable := mv.DB.Tables[TableTasks]
	descriptionField := taskTable.IndexOfField(FieldDescription)
	projectField := taskTable.IndexOfField(FieldPrimaryGoal)
	spanField := taskTable.IndexOfField(FieldSpan)
	statusField := taskTable.IndexOfField(FieldStatus)

	active, err := mv.queryAllTasks(StatusPlanned, StatusActive)
	if err != nil {
		return err
	}
	mv.tasks = map[string]([][]types.Entry){}
	for _, task := range active {
		span := task[spanField].Format("")
		// qurater scope tasks are good to keep an eye on, but to keep the
		// UX simple let's lump then in with the tasks for "this month"
		if span == SpanQuarter {
			span = SpanMonth
		}
		// If the task has already been started then mark it as active for today
		if task[statusField].Format("") == "Active" {
			span = SpanDay
		}
		if mv.Mode == MainViewModeListCycles {
			task, err = mv.retrieveAttentionCycle(taskTable, task)
			if err != nil {
				return err
			}
		}
		mv.tasks[span] = append(mv.tasks[span], task)
	}

	pending, err := mv.queryAllTasks(StatusPending)
	if err != nil {
		return err
	}
	for _, task := range pending {
		if mv.Mode == MainViewModeListCycles {
			task, err = mv.retrieveAttentionCycle(taskTable, task)
			if err != nil {
				return err
			}
		}
		mv.tasks[SpanPending] = append(mv.tasks[SpanPending], task)
	}
	for span := range mv.tasks {
		sort.Slice(mv.tasks[span], func(i, j int) bool {
			iRes := mv.tasks[span][i][projectField].Format("")
			jRes := mv.tasks[span][j][projectField].Format("")

			iDesc := mv.tasks[span][i][descriptionField].Format("")
			jDesc := mv.tasks[span][j][descriptionField].Format("")

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

	mv.log = [][]types.Entry{}
	if oy+cy >= len(mv.tasks[mv.span]) {
		return nil
	}
	selectedTask := mv.tasks[mv.span][oy+cy]
	resp, err := mv.queryLogs(selectedTask)
	if err != nil {
		return err
	}
	mv.log = resp.Entries
	return nil
}

func (mv *MainView) queryAllTasks(status ...string) ([][]types.Entry, error) {
	statusMap := map[string]bool{}
	for _, s := range status {
		statusMap[s] = true
	}
	taskTable := mv.DB.Tables[TableTasks]
	resp, err := taskTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.InFilter{
				Field:     FieldStatus,
				Col:       taskTable.IndexOfField(FieldStatus),
				Formatted: statusMap,
			},
		},
		OrderBy: FieldDescription,
	})

	if err != nil {
		return nil, err
	}
	entries := [][]types.Entry{}
	for _, entry := range resp.Entries {
		if IsGoalCycle(taskTable, entry) || IsCompositeTask(taskTable, entry) {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (mv *MainView) queryLogs(task []types.Entry) (*types.Response, error) {
	taskTable := mv.DB.Tables[TableTasks]
	logTable := mv.DB.Tables[TableLog]
	return logTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldTask,
				Col:       logTable.IndexOfField(FieldTask),
				Formatted: task[taskTable.IndexOfField(FieldDescription)].Format(""),
			},
		},
		OrderBy: FieldBegin,
		Dec:     true,
	})
}

func (mv *MainView) retrieveAttentionCycle(table *types.Table, task []types.Entry) ([]types.Entry, error) {
	orig := task
	seen := map[string]bool{}
	for {
		pk := task[table.Primary()].Format("")
		if seen[pk] {
			// hit a cycle
			return orig, nil
		}
		if IsAttentionCycle(table, task) {
			return task, nil
		}
		seen[pk] = true
		parent := task[table.IndexOfField(FieldPrimaryGoal)].Format("")
		resp, err := table.Query(types.QueryParams{
			Filters: []types.Filter{
				&ui.EqualFilter{
					Field:     FieldDescription,
					Col:       table.IndexOfField(FieldDescription),
					Formatted: parent,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Entries) < 1 {
			return orig, nil
		}
		task = resp.Entries[0]
	}
}

func (mv *MainView) switchModes(g *gocui.Gui, v *gocui.View) error {
	switch mv.Mode {
	case MainViewModeListBar:
		mv.Mode = MainViewModeListCycles
	case MainViewModeListCycles:
		mv.Mode = MainViewModeListBar
	}
	return mv.refreshView(g)
}

func (mv *MainView) queryActiveAndHabitualTasks() (*types.Response, error) {
	taskTable := mv.DB.Tables[TableTasks]
	return taskTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.InFilter{
				Field: FieldStatus,
				Col:   taskTable.IndexOfField(FieldStatus),
				Formatted: map[string]bool{
					StatusActive:   true,
					StatusHabitual: true,
				},
			},
		},
	})
}

func (mv *MainView) queryPlans(taskPKs []string) (*types.Response, error) {
	taskCols := map[string]bool{}
	for _, task := range taskPKs {
		taskCols[fmt.Sprintf("tasks %s", task)] = true
	}
	assertionsTable := mv.DB.Tables[TableAssertions]
	return assertionsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.InFilter{
				Field:     FieldArg0,
				Col:       assertionsTable.IndexOfField(FieldArg0),
				Formatted: taskCols,
			},
			&ui.EqualFilter{
				Field:     FieldARelation,
				Col:       assertionsTable.IndexOfField(FieldARelation),
				Formatted: ".Plan",
			},
		},
	})
}

func (mv *MainView) queryDayPlan() ([]types.Entry, error) {
	taskTable := mv.DB.Tables[TableTasks]
	resp, err := taskTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldAction,
				Col:       taskTable.IndexOfField(FieldAction),
				Formatted: "Plan",
			},
			&ui.EqualFilter{
				Field:     FieldDirect,
				Col:       taskTable.IndexOfField(FieldDirect),
				Formatted: "today",
			},
			&ui.EqualFilter{
				Field:     FieldSpan,
				Col:       taskTable.IndexOfField(FieldSpan),
				Formatted: "Day",
			},
			&ui.EqualFilter{
				Field:     FieldStatus,
				Col:       taskTable.IndexOfField(FieldStatus),
				Formatted: "Active",
			},
		},
		OrderBy: FieldStart,
		Dec:     true,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Entries) == 0 {
		return nil, fmt.Errorf("did not find a plan for today")
	}
	return resp.Entries[0], nil
}

func (mv *MainView) populateToday() error {
	// gather active and habitual tasks
	// gather each plan for those tasks
	// show an item if it is a plan or if it is an active leaf task with no plans
	// save contents
	taskTable := mv.DB.Tables[TableTasks]
	assertionsTable := mv.DB.Tables[TableAssertions]
	tasks, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return err
	}

	allTasks := []string{}
	task2children := map[string]([][]types.Entry){}
	task2plans := map[string][]string{}

	for _, task := range tasks.Entries {
		allTasks = append(allTasks, task[taskTable.Primary()].Format(""))
		parent := task[taskTable.IndexOfField(FieldPrimaryGoal)].Format("")
		task2children[parent] = append(task2children[parent], task)
	}

	plans, err := mv.queryPlans(allTasks)
	if err != nil {
		return err
	}
	for _, plan := range plans.Entries {
		planString := plan[assertionsTable.IndexOfField(FieldArg1)].Format("")
		// only include active plans though we query for all plans here because they may be useful later
		if strings.HasPrefix(planString, "[x] ") {
			continue
		}
		task := plan[assertionsTable.IndexOfField(FieldArg0)].Format("")[len("tasks "):]

		task2plans[task] = append(task2plans[task], planString)
	}
	dayPlan, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	items := []string{}
	for _, task := range tasks.Entries {
		pk := task[taskTable.Primary()].Format("")
		status := task[taskTable.IndexOfField(FieldStatus)].Format("")
		if status != "Active" || len(task2children[pk]) != 0 || len(task2plans[pk]) != 0 {
			continue
		}
		action := task[taskTable.IndexOfField(FieldAction)].Format("")
		direct := task[taskTable.IndexOfField(FieldDirect)].Format("")
		indirect := task[taskTable.IndexOfField(FieldIndirect)].Format("")
		// no need for self reference here
		if action == "Plan" && direct == "today" && indirect == "" {
			continue
		}
		items = append(items, fmt.Sprintf("[ ] %s", pk))
	}
	for _, taskPlans := range task2plans {
		for _, plan := range taskPlans {
			if !strings.HasPrefix(plan, "[ ] ") {
				plan = "[ ] " + plan
			}
			items = append(items, plan)
		}
	}
	// TODO Should only add the delta from what is already there
	for ix, item := range items {
		// pk doesn't really matter here so using a random integer
		pk := fmt.Sprintf("%d", rand.Int63())
		fields := map[string]string{
			FieldArg0:      fmt.Sprintf("tasks %s", dayPlan[taskTable.Primary()].Format("")),
			FieldArg1:      item,
			FieldARelation: ".To Plan", // In a breakdown of Do Today, Do Tomorrow, & To Plan we add to the end
			FieldOrder:     fmt.Sprintf("%d", ix),
		}
		err := assertionsTable.InsertWithFields(pk, fields)
		if err != nil {
			return err
		}
	}
	return mv.save()
}

func (mv *MainView) refreshTasks(g *gocui.Gui, v *gocui.View) error {
	err := exec.Command("jql-timedb-autofill").Run()
	if err != nil {
		return err
	}
	err = mv.load(g)
	if err != nil {
		return err
	}
	err = mv.populateToday()
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}
