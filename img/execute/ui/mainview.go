package ui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

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
	var store storage.Store
	if strings.HasSuffix(path, ".json") {
		store = &storage.JSONStore{}
	} else {
		return nil, fmt.Errorf("unknown file type")
	}
	mapper, err := osm.NewObjectStoreMapper(store)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		return nil, err
	}
	mv := &MainView{
		OSM: mapper,
		DB:  db,

		path: path,

		tasks: map[string]([][]types.Entry){},
		span:  SpanDay,
	}
	return mv, mv.refreshView(g)
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
	tasks.Highlight = true

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
	err = g.SetKeybinding(TasksView, 'q', gocui.ModNone, mv.switchToJQL)
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
	err = g.SetKeybinding(TasksView, gocui.KeySpace, gocui.ModNone, mv.markToday)
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

	active, err := mv.queryAllTasks(StatusActive)
	if err != nil {
		return err
	}
	mv.tasks = map[string]([][]types.Entry){}
	for _, task := range active.Entries {
		logs, err := mv.queryLogs(task)
		if err != nil {
			return err
		}
		span := task[spanField].Format("")
		// If the task has already been started then mark it as active for today
		if len(logs.Entries) != 0 {
			span = SpanDay
		}
		mv.tasks[span] = append(mv.tasks[span], task)
	}

	pending, err := mv.queryAllTasks(StatusPending)
	if err != nil {
		return err
	}
	for _, task := range pending.Entries {
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

func (mv *MainView) queryAllTasks(status string) (*types.Response, error) {
	taskTable := mv.DB.Tables[TableTasks]
	return taskTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldStatus,
				Col:       taskTable.IndexOfField(FieldStatus),
				Formatted: status,
			},
		},
		OrderBy: FieldDescription,
	})
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

func (mv *MainView) markToday(g *gocui.Gui, v *gocui.View) error {
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

	if len(mv.log) == 0 {
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")), selectedTask, true)
		if err != nil {
			return err
		}
	} else {
		log0 := mv.log[0]
		begin := log0[logTable.IndexOfField(FieldBegin)].Format("")
		end := log0[logTable.IndexOfField(FieldEnd)].Format("")
		if begin != end || len(mv.log) > 1 {
			return nil
		}
		err = logTable.Delete(log0[logTable.Primary()].Format(""))
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
