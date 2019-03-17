package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
)

// MainViewMode is the current mode of the MainView.
// It determines which subview processes inputs.
type MainViewMode int

const (
	// MainViewModeTable is the mode for standard table
	// navigation, filtering, ordering, &c
	MainViewModeTable MainViewMode = iota
	// MainViewModePrompt is for when the user is being
	// prompted to enter information
	MainViewModePrompt
	// MainViewModeAlert is for when the user is being
	// shown an alert in the prompt window
	MainViewModeAlert
	// MainViewModeEdit is for when the user is editing
	// the value of a single cell
	MainViewModeEdit
)

// A MainView is the overall view of the table including headers,
// prompts, &c. It will also be responsible for managing differnt
// interaction modes if jql supports those.
type MainView struct {
	path string

	OSM     *osm.ObjectStoreMapper
	DB      *types.Database
	Table   *types.Table
	Params  types.QueryParams
	columns []string
	// TODO map[string]types.Entry and []types.Entry could both
	// be higher-level types (e.g. VerboseRow and Row)
	entries [][]types.Entry

	TableView *TableView
	Mode      MainViewMode

	switching  bool // on when transitioning modes has not yet been acknowleged by Layout
	alert      string
	promptText string
	tableName  string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(path, start string) (*MainView, error) {
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
		path: path,
		OSM:  mapper,
		DB:   db,
	}
	return mv, mv.loadTable(start)
}

// findTable takes in a user-provided table name and returns
// either that name if it's an exact match for a table, or
// the first table to match the provided prefix, or an error if no
// table matches
func (mv *MainView) findTable(t string) (string, error) {
	_, ok := mv.DB.Tables[t]
	if ok {
		return t, nil
	}
	for name, _ := range mv.DB.Tables {
		if strings.HasPrefix(name, t) {
			return name, nil
		}
	}
	return "", fmt.Errorf("unknown table: %s", t)
}

// loadTable displays the named table in the main table view
func (mv *MainView) loadTable(t string) error {
	tName, err := mv.findTable(t)
	if err != nil {
		return err
	}
	table := mv.DB.Tables[tName]
	mv.Table = table
	mv.tableName = tName
	// TODO would be good to preserve params per table
	mv.Params.OrderBy = ""
	mv.Params.Filters = []types.Filter{}
	columns := []string{}
	widths := []int{}
	for _, column := range table.Columns {
		if strings.HasPrefix(column, "_") {
			// TODO note these to skip the values as well
			continue
		}
		widths = append(widths, 40)
		columns = append(columns, column)
	}
	mv.TableView = &TableView{
		Values: [][]string{},
		Widths: widths,
	}
	mv.columns = columns
	return mv.updateTableViewContents()
}

// Layout returns the gocui object
func (mv *MainView) Layout(g *gocui.Gui) error {
	switching := mv.switching
	mv.switching = false

	// TODO hide prompt if not in prompt mode or alert mode
	maxX, maxY := g.Size()
	v, err := g.SetView("table", 0, 0, maxX-2, maxY-3)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editable = true
		v.Editor = mv
	}
	v.Clear()
	if err := mv.TableView.WriteContents(v); err != nil {
		return err
	}
	prompt, err := g.SetView("prompt", 0, maxY-3, maxX-2, maxY-1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		prompt.Editable = true
		prompt.Editor = &PromptHandler{
			Callback: mv.promptExit,
		}
	}
	if switching {
		prompt.Clear()
	}
	switch mv.Mode {
	case MainViewModeTable:
		if _, err := g.SetCurrentView("table"); err != nil {
			return err
		}
		g.Cursor = false
	case MainViewModeAlert:
		if _, err := g.SetCurrentView("table"); err != nil {
			return err
		}
		g.Cursor = false
		fmt.Fprintf(prompt, mv.alert)
	case MainViewModePrompt:
		if _, err := g.SetCurrentView("prompt"); err != nil {
			return err
		}
		g.Cursor = true
		prompt.Write([]byte(mv.promptText))
		prompt.MoveCursor(len(mv.promptText), 0, true)
		mv.promptText = ""
	case MainViewModeEdit:
		if _, err := g.SetCurrentView("prompt"); err != nil {
			return err
		}
		g.Cursor = true
		prompt.Write([]byte(mv.promptText))
		prompt.MoveCursor(len(mv.promptText), 0, true)
		mv.promptText = ""
	}
	return nil
}

// newEntry prompts the user for the pk to a new entry and
// attempts to add an entry with that key
// TODO should just create an entry if using uuids
func (mv *MainView) newEntry() {
	mv.promptText = "create-new-entry "
	mv.switchMode(MainViewModePrompt)
}

// switchMode sets the main view's mode to the new mode and sets
// the switching flag so that Layout is aware of the transition
func (mv *MainView) switchMode(new MainViewMode) {
	mv.switching = true
	mv.Mode = new
}

// saveContents asks the osm to save the current contents to disk
func (mv *MainView) saveContents() error {
	f, err := os.OpenFile(mv.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	err = mv.OSM.Dump(mv.DB, f)
	if err != nil {
		return err
	}
	return fmt.Errorf("Wrote %s", mv.path)
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if mv.Mode == MainViewModeAlert {
		mv.switchMode(MainViewModeTable)
	}

	var err error
	defer func() {
		if err != nil {
			mv.alert = err.Error()
			mv.switchMode(MainViewModeAlert)
		}
	}()

	switch key {
	case gocui.KeyArrowRight:
		mv.TableView.Move(DirectionRight)
	case gocui.KeyArrowUp:
		mv.TableView.Move(DirectionUp)
	case gocui.KeyArrowLeft:
		mv.TableView.Move(DirectionLeft)
	case gocui.KeyArrowDown:
		mv.TableView.Move(DirectionDown)
	case gocui.KeyEnter:
		mv.switchMode(MainViewModeEdit)
		row, column := mv.TableView.GetSelected()
		mv.promptText = mv.TableView.Values[row][column]
	}

	switch ch {
	case 'r':
		mv.switchMode(MainViewModeEdit)
		mv.promptText = ""
	case 'l':
		mv.TableView.Move(DirectionRight)
	case 'k':
		mv.TableView.Move(DirectionUp)
	case 'h':
		mv.TableView.Move(DirectionLeft)
	case 'j':
		mv.TableView.Move(DirectionDown)
	case 'g':
		err = mv.goToSelectedValue()
	case 'G':
		err = mv.goFromSelectedValue()
	case 'f':
		row, col := mv.TableView.GetSelected()
		filterTarget := mv.entries[row][col].Format("")
		mv.Params.Filters = append(mv.Params.Filters, &EqualFilter{
			Col:       col,
			Formatted: filterTarget,
		})
		err = mv.updateTableViewContents()
	case 'q':
		if len(mv.Params.Filters) > 0 {
			mv.Params.Filters = mv.Params.Filters[:len(mv.Params.Filters)-1]
		}
		err = mv.updateTableViewContents()
	case 'Q':
		mv.Params.Filters = []types.Filter{}
		err = mv.updateTableViewContents()
	case 'd':
		err = mv.deleteSelectedRow()
		if err != nil {
			return
		}
		err = mv.updateTableViewContents()
	case 'D':
		err = mv.duplicateSelectedRow()
		if err != nil {
			return
		}
		err = mv.updateTableViewContents()
	case '\'':
		mv.switchMode(MainViewModePrompt)
		mv.promptText = "switch-table "
	case ':':
		mv.switchMode(MainViewModePrompt)
	case 'o':
		_, col := mv.TableView.GetSelected()
		mv.Params.OrderBy = mv.columns[col]
		mv.Params.Dec = false
		err = mv.updateTableViewContents()
	case 'O':
		_, col := mv.TableView.GetSelected()
		mv.Params.OrderBy = mv.columns[col]
		mv.Params.Dec = true
		err = mv.updateTableViewContents()
	case 'i':
		err = mv.incrementSelected(1)
	case 'I':
		err = mv.incrementSelected(-1)
	case 's':
		err = mv.saveContents()
	case 'n':
		mv.newEntry()
	case 'w':
		err = mv.openCellInWindow()
	}
}

func (mv *MainView) updateTableViewContents() error {
	mv.TableView.Values = [][]string{}
	// NOTE putting this here to support swapping columns later
	header := []string{}
	for _, col := range mv.columns {
		if mv.Params.OrderBy == col {
			if mv.Params.Dec {
				col += " ^"
			} else {
				col += " v"
			}
		}
		header = append(header, col)
	}
	mv.TableView.Header = header

	entries, err := mv.Table.Query(mv.Params)
	if err != nil {
		return err
	}
	mv.entries = entries
	for _, row := range mv.entries {
		// TODO ignore hidden columns
		formatted := []string{}
		for _, entry := range row {
			// TODO extract actual formatting
			formatted = append(formatted, entry.Format(""))
		}
		mv.TableView.Values = append(mv.TableView.Values, formatted)
	}
	return nil
}

func (mv *MainView) promptExit(contents string, finish bool, err error) {
	current := mv.Mode
	if !finish {
		return
	}
	defer func() {
		if err != nil {
			mv.switchMode(MainViewModeAlert)
			mv.alert = err.Error()
		} else {
			mv.switchMode(MainViewModeTable)
		}
	}()
	if err != nil {
		return
	}
	switch current {
	case MainViewModeEdit:
		row, column := mv.TableView.GetSelected()
		primary := mv.Table.Primary()
		key := mv.entries[row][primary].Format("")
		err = mv.Table.Update(key, mv.Table.Columns[column], contents)
		if err != nil {
			return
		}
		err = mv.updateTableViewContents()
		return
	case MainViewModePrompt:
		parts := strings.Split(contents, " ")
		if len(parts) == 0 {
			return
		}
		command := parts[0]
		switch command {
		case "switch-table":
			if len(parts) != 2 {
				err = fmt.Errorf("switch-table takes 1 arg")
				return
			}
			err = mv.loadTable(parts[1])
			return
		case "create-new-entry":
			if len(parts) == 0 {
				err = fmt.Errorf("create-new-entry takes at least")
				return
			}
			newPK := strings.Join(parts[1:], " ")
			err = mv.Table.Insert(newPK)
			if err != nil {
				return
			}
			for _, filter := range mv.Params.Filters {
				col, formatted := filter.Example()
				if col == -1 {
					// TODO should unapply all filters here
					continue
				}
				err = mv.Table.Update(newPK, mv.Table.Columns[col], formatted)
				if err != nil {
					return
				}
			}
			err = mv.updateTableViewContents()
			return
		default:
			err = fmt.Errorf("unknown command: %s", contents)
		}
	}
}

func (mv *MainView) goToSelectedValue() error {
	row, col := mv.TableView.GetSelected()
	entry := mv.entries[row][col]
	// TODO leaky abstraction. Maybe better to support
	// an interface method for detecting foreigns
	foreign, ok := entry.(types.ForeignKey)
	if !ok {
		return fmt.Errorf("must select a foreign key")
	}
	err := mv.loadTable(foreign.Table)
	if err != nil {
		return err
	}
	primary := mv.DB.Tables[foreign.Table].Primary()
	mv.Params.Filters = []types.Filter{
		&EqualFilter{
			Col:       primary,
			Formatted: foreign.Key,
		},
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) goFromSelectedValue() error {
	row, _ := mv.TableView.GetSelected()
	selected := mv.entries[row][mv.Table.Primary()]
	for name, table := range mv.DB.Tables {
		col := table.HasForeign(mv.tableName)
		if col == -1 {
			continue
		}
		err := mv.loadTable(name)
		if err != nil {
			return err
		}
		mv.Params.Filters = []types.Filter{
			&EqualFilter{
				Col:       col,
				Formatted: selected.Format(""),
			},
		}
		return mv.updateTableViewContents()
	}
	return fmt.Errorf("no tables found with corresponding foreign key: %s", selected)
}

func (mv *MainView) incrementSelected(amt int) error {
	row, col := mv.TableView.GetSelected()
	entry := mv.entries[row][col]
	key := mv.entries[row][mv.Table.Primary()].Format("")
	// TODO leaky abstraction
	switch typed := entry.(type) {
	case types.ForeignKey:
		ftable := mv.DB.Tables[typed.Table]
		// TODO not cache mapping
		fentries, err := ftable.Query(types.QueryParams{
			OrderBy: ftable.Columns[ftable.Primary()],
		})
		if err != nil {
			return err
		}
		index := map[string]int{}
		for i, fentry := range fentries {
			index[fentry[ftable.Primary()].Format("")] = i
		}
		next := (index[entry.Format("")] + 1) % len(fentries)
		err = mv.Table.Update(key, mv.Table.Columns[col], fentries[next][ftable.Primary()].Format(""))
		if err != nil {
			return err
		}
	default:
		// TODO should use an Update so table can modify any necessary internals
		new, err := mv.Table.Entries[key][col].Add(amt)
		if err != nil {
			return err
		}
		mv.Table.Entries[key][col] = new
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) openCellInWindow() error {
	row, col := mv.TableView.GetSelected()
	entry := mv.entries[row][col]
	cmd := exec.Command("open", entry.Format(""))
	return cmd.Run()
}

func (mv *MainView) deleteSelectedRow() error {
	row, _ := mv.TableView.GetSelected()
	key := mv.entries[row][mv.Table.Primary()].Format("")
	return mv.Table.Delete(key)
}

func (mv *MainView) duplicateSelectedRow() error {
	row, _ := mv.TableView.GetSelected()
	old := mv.entries[row]
	primaryIndex := mv.Table.Primary()
	key := old[primaryIndex].Format("")
	// TODO look for number patterns inside the description and
	// try to increment those instead
	ordinal := 0
	newKey := ""
	for {
		ordinal += 1
		newKey = fmt.Sprintf("%s (%d)", key, ordinal)
		if _, ok := mv.Table.Entries[newKey]; !ok {
			break
		}
	}
	err := mv.Table.Insert(newKey)
	if err != nil {
		return err
	}
	for i, oldValue := range old {
		if i == primaryIndex {
			continue
		}
		err = mv.Table.Update(newKey, mv.Table.Columns[i], oldValue.Format(""))
		if err != nil {
			return err
		}
	}
	return nil
}
