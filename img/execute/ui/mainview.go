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

// A MainView is the overall view including a resource list
// and a detailed view of the current resource
type MainView struct {
	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	Mode MainViewMode

	items [][]types.Entry
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
	items, err := g.SetView(ItemsView, 0, 0, (maxX*3)/4, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	items.Clear()
	g.SetCurrentView(ItemsView)
	log, err := g.SetView(LogView, (maxX*3/4)+1, 0, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	log.Clear()
	items.SelBgColor = gocui.ColorWhite
	items.SelFgColor = gocui.ColorBlack
	items.Highlight = true

	for _, desc := range mv.tabulatedItems() {
		fmt.Fprintf(items, "%s\n", desc)
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

func (mv *MainView) tabulatedItems() []string {
	itemTable := mv.DB.Tables[TableItems]
	resourceField := itemTable.IndexOfField(FieldResource)
	descriptionField := itemTable.IndexOfField(FieldDescription)

	// 10 char buffer
	buffer := 10
	maxChars := buffer
	for _, item := range mv.items {
		itemChars := len(item[resourceField].Format("")) + buffer
		if itemChars > maxChars {
			maxChars = itemChars
		}
	}

	toret := []string{}

	for _, item := range mv.items {
		itemBuffer := maxChars - len(item[resourceField].Format(""))
		toret = append(toret,
			fmt.Sprintf("%s%s%s", item[resourceField].Format(""), strings.Repeat(" ", itemBuffer),
				item[descriptionField].Format("")))
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
	err := g.SetKeybinding(ItemsView, 'k', gocui.ModNone, mv.cursorUp)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 'j', gocui.ModNone, mv.cursorDown)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 's', gocui.ModNone, mv.saveContents)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(ItemsView, gocui.KeyEnter, gocui.ModNone, mv.logTime); err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 'w', gocui.ModNone, mv.openLink)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 'i', gocui.ModNone, mv.markSatisfied)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 'q', gocui.ModNone, mv.switchToJQL)
	if err != nil {
		return err
	}

	return nil
}

func (mv *MainView) markSatisfied(g *gocui.Gui, v *gocui.View) error {
	// TODO getting selected item is very common. Should factor out.
	itemTable := mv.DB.Tables[TableItems]
	var cy, oy int
	view, err := g.View(ItemsView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedItem := mv.items[oy+cy]
	pk := selectedItem[itemTable.IndexOfField(FieldDescription)].Format("")

	new, err := selectedItem[itemTable.IndexOfField(FieldStatus)].Add(1)
	if err != nil {
		return err
	}
	itemTable.Entries[pk][itemTable.IndexOfField(FieldStatus)] = new
	err = mv.saveContents(g, v)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) openLink(g *gocui.Gui, v *gocui.View) error {
	itemTable := mv.DB.Tables[TableItems]
	var cy, oy int
	view, err := g.View(ItemsView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedItem := mv.items[oy+cy]
	cmd := exec.Command("open", selectedItem[itemTable.IndexOfField(FieldLink)].Format(""))
	return cmd.Run()
}

func (mv *MainView) logTime(g *gocui.Gui, v *gocui.View) error {
	itemTable := mv.DB.Tables[TableItems]
	logTable := mv.DB.Tables[TableLog]
	var cy, oy int
	view, err := g.View(ItemsView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedItem := mv.items[oy+cy]

	// XXX this is a really janky way to check the value of the time entry
	// and create the next valid entry
	if len(mv.log) == 0 {
		err = mv.newTime(g, fmt.Sprintf("%s (0001)", selectedItem[itemTable.IndexOfField(FieldDescription)].Format("")), selectedItem)
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
		err = mv.newTime(g, newPK, selectedItem)
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

func (mv *MainView) newTime(g *gocui.Gui, pk string, selectedItem []types.Entry) error {
	itemTable := mv.DB.Tables[TableItems]
	logTable := mv.DB.Tables[TableLog]
	err := logTable.Insert(pk)
	if err != nil {
		return err
	}
	err = logTable.Update(pk, FieldBegin, "")
	if err != nil {
		return err
	}
	return logTable.Update(pk, FieldItem, selectedItem[itemTable.IndexOfField(FieldDescription)].Format(""))
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

	args := []string{JQLName, mv.path, TableItems}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	itemTable, ok := mv.DB.Tables[TableItems]
	if !ok {
		return fmt.Errorf("expected resources table to exist")
	}
	resp, err := itemTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldStatus,
				Col:       itemTable.IndexOfField(FieldStatus),
				Formatted: StatusPending,
			},
		},
		OrderBy: FieldDescription,
	})
	if err != nil {
		return err
	}
	mv.items = resp.Entries

	descriptionField := itemTable.IndexOfField(FieldDescription)
	resourceField := itemTable.IndexOfField(FieldResource)

	sort.Slice(mv.items, func(i, j int) bool {
		iRes := mv.items[i][resourceField].Format("")
		jRes := mv.items[j][resourceField].Format("")

		iDesc := mv.items[i][descriptionField].Format("")
		jDesc := mv.items[j][descriptionField].Format("")

		return (iRes < jRes) || ((iRes == jRes) && iDesc < jDesc)
	})

	var cy, oy int
	view, err := g.View(ItemsView)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	} else if err == nil {
		_, oy = view.Origin()
		_, cy = view.Cursor()
	}

	selectedItem := mv.items[oy+cy]
	logTable, ok := mv.DB.Tables[TableLog]
	if !ok {
		return fmt.Errorf("Expected log table to exist")
	}
	resp, err = logTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldItem,
				Col:       logTable.IndexOfField(FieldItem),
				Formatted: selectedItem[itemTable.IndexOfField(FieldDescription)].Format(""),
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
