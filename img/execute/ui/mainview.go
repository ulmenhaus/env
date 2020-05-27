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

	tasks [][]types.Entry
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
	}
	return mv, mv.refreshView(g)
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	tasks, err := g.SetView(TasksView, 0, 0, (maxX*3)/4, maxY-1)
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
	for _, task := range mv.tasks {
		taskChars := len(task[projectField].Format("")) + buffer
		if taskChars > maxChars {
			maxChars = taskChars
		}
	}

	toret := []string{}

	for _, task := range mv.tasks {
		taskBuffer := maxChars - len(task[projectField].Format(""))
		toret = append(toret,
			fmt.Sprintf("%s%s%s", task[projectField].Format(""), strings.Repeat(" ", taskBuffer),
				task[descriptionField].Format("")))
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
	err = g.SetKeybinding(TasksView, 'i', gocui.ModNone, mv.markSatisfied)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TasksView, 'q', gocui.ModNone, mv.switchToJQL)
	if err != nil {
		return err
	}

	return nil
}

func (mv *MainView) markSatisfied(g *gocui.Gui, v *gocui.View) error {
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

	selectedTask := mv.tasks[oy+cy]
	pk := selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")

	new, err := selectedTask[taskTable.IndexOfField(FieldStatus)].Add(1)
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

	selectedTask := mv.tasks[oy+cy]
	cmd := exec.Command("open", selectedTask[taskTable.IndexOfField(FieldLink)].Format(""))
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

	selectedTask := mv.tasks[oy+cy]

	// XXX this is a really janky way to check the value of the time entry
	// and create the next valid entry
	if len(mv.log) == 0 {
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedTask[taskTable.IndexOfField(FieldDescription)].Format("")), selectedTask)
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
		err = mv.newTime(g, newPK, selectedTask)
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

func (mv *MainView) newTime(g *gocui.Gui, pk string, selectedTask []types.Entry) error {
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

func (mv *MainView) refreshView(g *gocui.Gui) error {
	taskTable, ok := mv.DB.Tables[TableTasks]
	if !ok {
		return fmt.Errorf("expected projects table to exist")
	}
	resp, err := taskTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldStatus,
				Col:       taskTable.IndexOfField(FieldStatus),
				Formatted: StatusActive,
			},
		},
		OrderBy: FieldDescription,
	})
	if err != nil {
		return err
	}
	allActive := resp.Entries

	descriptionField := taskTable.IndexOfField(FieldDescription)
	projectField := taskTable.IndexOfField(FieldPrimaryGoal)
	indirectField := taskTable.IndexOfField(FieldIndirect)
	actionField := taskTable.IndexOfField(FieldAction)
	possessors := map[string]bool{}
	for _, active := range allActive {
		possessors[active[projectField].Format("")] = true
	}
	// Ignore higher level tasks (e.g. projects) which have active tasks
	mv.tasks = [][]types.Entry{}
	for _, active := range allActive {
		if possessors[active[descriptionField].Format("")] {
			continue
		}
		indirect := active[indirectField].Format("")
		// Likewise ignore projects which have autogenerated tasks
		if indirect == HabitRegularity || indirect == HabitIncrementality || indirect == HabitBreakdown {
			continue
		}
		action := active[actionField].Format("")
		// And keywords indicating goals
		if action == ActionExtend || action == ActionImprove || action == ActionSustain {
			continue
		}
		mv.tasks = append(mv.tasks, active)
	}

	sort.Slice(mv.tasks, func(i, j int) bool {
		iRes := mv.tasks[i][projectField].Format("")
		jRes := mv.tasks[j][projectField].Format("")

		iDesc := mv.tasks[i][descriptionField].Format("")
		jDesc := mv.tasks[j][descriptionField].Format("")

		return (iRes < jRes) || ((iRes == jRes) && iDesc < jDesc)
	})

	var cy, oy int
	view, err := g.View(TasksView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedTask := mv.tasks[oy+cy]
	logTable, ok := mv.DB.Tables[TableLog]
	if !ok {
		return fmt.Errorf("Expected log table to exist")
	}
	resp, err = logTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldTask,
				Col:       logTable.IndexOfField(FieldTask),
				Formatted: selectedTask[taskTable.IndexOfField(FieldDescription)].Format(""),
			},
		},
		OrderBy: FieldBegin,
		Dec:     true,
	})
	if err != nil {
		return err
	}
	mv.log = resp.Entries
	return nil
}
