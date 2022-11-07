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

	// today
	cachedTodayTasks []string
	today            []DayItem
	today2item       map[string]DayItemMeta
	ix2item          map[int]DayItem
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
	mv.span = Today
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

	for _, desc := range mv.tabulatedTasks(g, tasks) {
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

func (mv *MainView) tabulatedTasks(g *gocui.Gui, v *gocui.View) []string {
	if mv.span == Today {
		wasNil := mv.cachedTodayTasks == nil
		mv.cachedTodayTasks = mv.todayTasks()
		if wasNil {
			mv.selectNextFreeTask(g, v)
		}
		return mv.cachedTodayTasks
	}
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

func (mv *MainView) todayTasks() []string {
	tasks := []string{}
	ix2item := map[int]DayItem{}
	currentBreak := ""
	for _, item := range mv.today {
		if item.Break != currentBreak {
			tasks = append(tasks, item.Break)
			currentBreak = item.Break
		}
		ix2item[len(tasks)] = item
		tasks = append(tasks, " "+item.Description)
	}
	mv.ix2item = ix2item
	return tasks
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
	err = g.SetKeybinding(TasksView, 'x', gocui.ModNone, mv.markTask)
	if err != nil {
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
	}
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
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	if mv.span == Today {
		item, ok := mv.ix2item[ix]
		if !ok {
			return nil
		}
		meta, ok := mv.today2item[item.Description]
		if !ok {
			return nil
		}
		return mv.goToPK(g, v, meta.TaskPK)
	} else {
		taskTable := mv.DB.Tables[TableTasks]
		selectedTask := mv.tasks[mv.span][ix]
		return mv.goToPK(g, v, selectedTask[taskTable.IndexOfField(FieldDescription)].Format(""))
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
	if mv.span != Today {
		if oy+cy < len(mv.tasks[mv.span]) {
			selectedTask := mv.tasks[mv.span][oy+cy]
			resp, err := mv.queryLogs(selectedTask)
			if err != nil {
				return err
			}
			mv.log = resp.Entries
		}
	}
	return mv.refreshToday()
}

func (mv *MainView) refreshToday() error {
	mv.today = []DayItem{}
	mv.today2item = map[string]DayItemMeta{}

	today, err := mv.queryDayPlan()
	if err != nil {
		return err
	}
	if today == nil {
		return nil
	}
	assertionsTable := mv.DB.Tables[TableAssertions]
	tasksTable := mv.DB.Tables[TableTasks]
	resp, err := assertionsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldArg0,
				Col:       assertionsTable.IndexOfField(FieldArg0),
				Formatted: fmt.Sprintf("tasks %s", today[tasksTable.Primary()].Format("")),
			},
			&ui.EqualFilter{
				Field:     FieldARelation,
				Col:       assertionsTable.IndexOfField(FieldARelation),
				Formatted: ".Do Today",
			},
		},
		OrderBy: FieldOrder,
	})
	if err != nil {
		return err
	}

	currentBreak := ""
	for _, entry := range resp.Entries {
		val := entry[assertionsTable.IndexOfField(FieldArg1)].Format("")
		if !strings.HasPrefix(val, "[") {
			currentBreak = val
			continue
		}
		mv.today = append(mv.today, DayItem{
			Description: val,
			Break:       currentBreak,
			PK:          entry[assertionsTable.Primary()].Format(""),
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
		return nil, nil
	}
	return resp.Entries[0], nil
}

func (mv *MainView) queryYesterday() ([]types.Entry, error) {
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
		},
		OrderBy: FieldStart,
		Dec:     true,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Entries) < 2 {
		return nil, fmt.Errorf("did not find a plan for yesterday")
	}
	return resp.Entries[1], nil
}

func (mv *MainView) queryExistingTasks(planPK string) (map[string]bool, error) {
	assertionsTable := mv.DB.Tables[TableAssertions]
	resp, err := assertionsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldArg0,
				Col:       assertionsTable.IndexOfField(FieldArg0),
				Formatted: fmt.Sprintf("tasks %s", planPK),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, entry := range resp.Entries {
		task := entry[assertionsTable.IndexOfField(FieldArg1)].Format("")
		if !strings.HasPrefix(task, "[ ] ") {
			continue
		}
		existing[task] = true
	}
	return existing, nil
}

func (mv *MainView) copyOldTasks() error {
	taskTable := mv.DB.Tables[TableTasks]
	assertionsTable := mv.DB.Tables[TableAssertions]

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

	todayPK := today[taskTable.Primary()].Format("")
	yesterdayPK := yesterday[taskTable.Primary()].Format("")

	todayBullets, err := assertionsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldArg0,
				Col:       assertionsTable.IndexOfField(FieldArg0),
				Formatted: fmt.Sprintf("tasks %s", todayPK),
			},
		},
	})
	if err != nil {
		return err
	}
	// short-circuit if today is already populated
	if len(todayBullets.Entries) > 0 {
		return nil
	}

	oldBullets, err := assertionsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldArg0,
				Col:       assertionsTable.IndexOfField(FieldArg0),
				Formatted: fmt.Sprintf("tasks %s", yesterdayPK),
			},
		},
	})
	if err != nil {
		return err
	}

	for _, oldBullet := range oldBullets.Entries {
		rel := oldBullet[assertionsTable.IndexOfField(FieldARelation)].Format("")
		val := oldBullet[assertionsTable.IndexOfField(FieldArg1)].Format("")
		order := oldBullet[assertionsTable.IndexOfField(FieldOrder)].Format("")

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
	taskTable := mv.DB.Tables[TableTasks]
	assertionsTable := mv.DB.Tables[TableAssertions]
	tasks, err := mv.queryActiveAndHabitualTasks()
	if err != nil {
		return nil, err
	}

	allTasks := []string{}
	task2children := map[string]([][]types.Entry){}
	task2plans := map[string][]DayItemMeta{}

	for _, task := range tasks.Entries {
		allTasks = append(allTasks, task[taskTable.Primary()].Format(""))
		parent := task[taskTable.IndexOfField(FieldPrimaryGoal)].Format("")
		task2children[parent] = append(task2children[parent], task)
	}

	plans, err := mv.queryPlans(allTasks)
	if err != nil {
		return nil, err
	}
	items := []DayItemMeta{}
	for _, plan := range plans.Entries {
		planString := plan[assertionsTable.IndexOfField(FieldArg1)].Format("")
		// only include active plans though we query for all plans here because they may be useful later
		if strings.HasPrefix(planString, "[x] ") {
			continue
		}
		if !strings.HasPrefix(planString, "[ ] ") {
			planString = "[ ] " + planString
		}
		task := plan[assertionsTable.IndexOfField(FieldArg0)].Format("")[len("tasks "):]

		task2plans[task] = append(task2plans[task], DayItemMeta{
			Description: planString,
			TaskPK:      task,
			AssertionPK: plan[assertionsTable.Primary()].Format(""),
		})
	}
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
	taskTable := mv.DB.Tables[TableTasks]
	assertionsTable := mv.DB.Tables[TableAssertions]

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
	dayPlanPK := dayPlan[taskTable.Primary()].Format("")
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
	_, oy := v.Origin()
	_, cy := v.Cursor()

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
	assertionsTable := mv.DB.Tables[TableAssertions]
	tasksTable := mv.DB.Tables[TableTasks]
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
	err := mv.save()
	if err != nil {
		return err
	}
	err = mv.cursorDown(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}
