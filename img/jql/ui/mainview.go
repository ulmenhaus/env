package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

// MainViewMode is the current mode of the MainView.
// It determines which subview processes inputs.
type MainViewMode int

type MacroResponseFilter struct {
	Field     string `json:"field"`
	Formatted string `json:"formatted"`
}

type MacroCurrentView struct {
	Table            string              `json:"table"`
	PKs              []string            `json:"pks"`
	PrimarySelection string              `json:"primary_selection"`
	Filter           MacroResponseFilter `json:"filter"`
	OrderBy          string              `json:"order_by"`
	OrderDec         bool                `json:"order_dec"`
}

type MacroInterface struct {
	Snapshot    string           `json:"snapshot"`
	CurrentView MacroCurrentView `json:"current_view"`
}

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
	// MainViewModeSearch is for when the user is typing
	// a search query into the view
	MainViewModeSearch
	// MainViewModeSelectBox is for when the user is selecting
	// from a collection of predefined values for a field
	MainViewModeSelectBox

	// MacroTable is the name of the standard table containing
	// macros
	MacroTable = "macros"
	// MacroLocationCol is the name of the column of the macros
	// table containing the location of the program to run
	MacroLocationCol = "Location"
)

// A MainView is the overall view of the table including headers,
// prompts, &c. It will also be responsible for managing differnt
// interaction modes if jql supports those.
type MainView struct {
	path string
	dbms api.JQL_DBMS

	OSM     *osm.ObjectStoreMapper
	Table   *types.Table
	Params  types.QueryParams
	columns []string
	colix   []int
	// TODO map[string]types.Entry and []types.Entry could both
	// be higher-level types (e.g. VerboseRow and Row)
	response *types.Response

	TableView *TableView
	Mode      MainViewMode

	switching     bool // on when transitioning modes has not yet been acknowleged by Layout
	alert         string
	promptText    string
	tableName     string
	searchText    string
	searchAll     bool // indicates if we search all fields or just this one
	selectOptions []string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(path, start string, mapper *osm.ObjectStoreMapper, dbms api.JQL_DBMS) (*MainView, error) {
	mv := &MainView{
		path: path,
		dbms: dbms,
		OSM:  mapper,
	}
	return mv, mv.loadTable(start)
}

// findTable takes in a user-provided table name and returns
// either that name if it's an exact match for a table, or
// the first table to match the provided prefix, or an error if no
// table matches
func (mv *MainView) findTable(t string) (string, error) {
	_, ok := mv.OSM.GetDB().Tables[t]
	if ok {
		return t, nil
	}
	for name, _ := range mv.OSM.GetDB().Tables {
		if strings.HasPrefix(name, t) {
			return name, nil
		}
	}
	return "", fmt.Errorf("unknown table: %s", t)
}

func (mv *MainView) maxWidths(t *types.Table) ([]int, error) {
	all, err := t.Query(types.QueryParams{})
	if err != nil {
		return nil, err
	}
	if len(all.Entries) == 0 {
		return nil, nil
	}
	max := make([]int, len(all.Entries[0]))
	for i := range all.Entries {
		for j, entry := range all.Entries[i] {
			chars := len(entry.Format(""))
			if max[j] < chars {
				max[j] = chars
			}
		}
	}
	return max, nil
}

// loadTable displays the named table in the main table view
func (mv *MainView) loadTable(t string) error {
	tName, err := mv.findTable(t)
	if err != nil {
		return err
	}
	table := mv.OSM.GetDB().Tables[tName]
	mv.Table = table
	mv.tableName = tName
	columns := []string{}
	colix := []int{}
	widths := []int{}
	max, err := mv.maxWidths(table)
	if err != nil {
		return err
	}
	for i, column := range table.Columns {
		columns = append(columns, column)
		if strings.HasPrefix(column, "_") {
			continue
		}
		width := 40
		if max != nil && max[i] < width {
			width = max[i]
		}
		widths = append(widths, width)
		colix = append(colix, i)
	}
	mv.TableView = &TableView{
		Values: [][]string{},
		Widths: widths,
		Selections: SelectionSet{
			Secondary: make(map[Coordinate]bool),
			Tertiary:  make(map[Coordinate]bool),
		},
	}
	mv.columns = columns
	mv.colix = colix
	// TODO would be good to preserve params per table
	mv.Params.OrderBy = mv.columns[0]
	mv.Params.Filters = []types.Filter{}
	mv.Params.Offset = uint(0)
	return mv.updateTableViewContents()
}

func (mv *MainView) filteredSelectOptions(g *gocui.Gui) []string {
	searchBox, err := g.SetCurrentView("searchBox")
	if err != nil {
		return []string{}
	}
	query := strings.TrimSuffix(searchBox.Buffer(), "\n")
	filtered := []string{}
	for _, pk := range mv.selectOptions {
		if strings.Contains(strings.ToLower(pk), strings.ToLower(query)) {
			filtered = append(filtered, pk)
		}
	}
	return filtered
}

// Layout returns the gocui object
func (mv *MainView) Layout(g *gocui.Gui) error {
	switching := mv.switching
	mv.switching = false

	// TODO hide prompt and header if not in prompt mode or alert mode
	maxX, maxY := g.Size()
	// HACK maxY - 8 is the max number of visible rows when header and prompt are present
	maxRows := uint(maxY - 8)
	if mv.Params.Limit != maxRows {
		mv.Params.Limit = maxRows
		if err := mv.updateTableViewContents(); err != nil {
			return err
		}
	}
	_, err := g.SetView("header", 0, 0, maxX-2, 3)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	v, err := g.SetView("table", 0, 3, maxX-2, maxY-5)
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
	location, err := g.SetView("location", 0, maxY-5, maxX-2, maxY-3)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	location.Clear()
	row, col := mv.SelectedEntry()
	var primarySelection types.Entry
	primarySelection = types.String("")
	if row < len(mv.response.Entries) {
		primarySelection = mv.response.Entries[row][mv.Table.Primary()]
	}
	location.Write([]byte(fmt.Sprintf("    L%d C%d           %s", row, col, primarySelection.Format(""))))
	if mv.Mode == MainViewModeSelectBox {
		selectBox, err := g.SetView("selectBox", maxX/2-30, maxY/2-10, maxX/2+30, maxY/2+10)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			selectBox.SelBgColor = gocui.ColorWhite
			selectBox.SelFgColor = gocui.ColorBlack
			selectBox.Highlight = true
			searchBox, err := g.SetView("searchBox", maxX/2-30, maxY/2-12, maxX/2+30, maxY/2-10)
			if err != nil {
				if err != gocui.ErrUnknownView {
					return err
				}
				searchBox.Editable = true
				g.SetKeybinding("searchBox", gocui.KeyEnter, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
					return mv.handleSelectInput(g, selectBox)
				})
				g.SetKeybinding("searchBox", gocui.KeyArrowUp, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
					return mv.cursorUp(g, selectBox)
				})
				g.SetKeybinding("searchBox", gocui.KeyArrowDown, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
					return mv.cursorDown(g, selectBox)
				})
			}
		}
	} else {
		for _, elem := range []string{"searchBox", "selectBox"} {
			err := g.DeleteView(elem)
			if err != nil && err != gocui.ErrUnknownView {
				return err
			}
		}
		for _, key := range []interface{}{gocui.KeyEnter, gocui.KeyArrowDown, gocui.KeyArrowUp} {
			err := g.DeleteKeybinding("searchBox", key, gocui.ModNone)
			if err != nil && err.Error() != "keybinding not found" {
				return err
			}

		}

	}
	switch mv.Mode {
	case MainViewModeSelectBox:
		selectBox, err := g.View("selectBox")
		if err != nil {
			return err
		}
		selectBox.Clear()
		_, err = selectBox.Write([]byte(strings.Join(mv.filteredSelectOptions(g), "\n")))
		if err != nil {
			return err
		}
	case MainViewModeTable:
		header, err := g.SetCurrentView("header")
		if err != nil {
			return err
		}
		header.Clear()
		header.Write(mv.headerContents())
		if _, err = g.SetCurrentView("table"); err != nil {
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
	case MainViewModeSearch:
		prompt.Clear()
		if mv.searchAll {
			prompt.Write([]byte{'?'})
		} else {
			prompt.Write([]byte{'/'})
		}
		prompt.Write([]byte(mv.searchText))
	}
	return nil
}

// newEntry prompts the user for the pk to a new entry and
// attempts to add an entry with that key
// TODO should just create an entry if using uuids
func (mv *MainView) newEntry(prefill bool) error {
	sample := ""
	if prefill {
		for _, filter := range mv.Params.Filters {
			suggestion, yes := filter.PrimarySuggestion()
			if !yes {
				continue
			}
			next, err := mv.nextAvailablePrimaryFromPattern(fmt.Sprintf("%s (0000)", suggestion))
			if err != nil {
				return err
			}
			sample = next
			break
		}
	}
	mv.promptText = fmt.Sprintf("create-new-entry %s", sample)
	mv.switchMode(MainViewModePrompt)
	return nil
}

// switchMode sets the main view's mode to the new mode and sets
// the switching flag so that Layout is aware of the transition
func (mv *MainView) switchMode(new MainViewMode) {
	mv.switching = true
	mv.Mode = new
}

// saveContents asks the osm to save the current contents to disk
func (mv *MainView) saveContents() error {
	err := mv.saveSilent()
	if err != nil {
		return err
	}
	return fmt.Errorf("Wrote %s", mv.path)
}

func (mv *MainView) saveSilent() error {
	return mv.OSM.StoreEntries()
}

func (mv *MainView) handleSelectInput(g *gocui.Gui, v *gocui.View) error {
	options := mv.filteredSelectOptions(g)
	_, cy := v.Cursor()
	_, oy := v.Origin()
	index := cy + oy
	var selected string
	if index >= len(options) {
		selected = options[len(options)-1]
	} else {
		selected = options[index]
	}
	mv.switchMode(MainViewModeTable)
	return mv.updateEntryValue(selected)
}

func (mv *MainView) handleSearchInput(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) error {
	if key == gocui.KeyEnter || key == gocui.KeyEsc {
		mv.searchText = ""
		mv.Mode = MainViewModeTable
		mv.switching = true
		return nil
	}
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.searchText) != 0 {
			mv.searchText = mv.searchText[:len(mv.searchText)-1]
		}
	} else if key == gocui.KeySpace {
		mv.searchText += " "
	} else {
		mv.searchText += string(ch)
	}
	// when we start search pagination should be reset
	mv.Params.Offset = uint(0)
	// When switching into search mode, the last filter added is the working
	// search filter

	col := mv.TableView.Selections.Primary.Column
	field := mv.Table.Columns[mv.TableView.Selections.Primary.Column]
	if mv.searchAll {
		col = -1
		field = "Any field"
	}
	mv.Params.Filters[len(mv.Params.Filters)-1] = &ContainsFilter{
		Field:     field,
		Col:       col,
		Formatted: mv.searchText,
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) triggerEdit() error {
	row, col := mv.SelectedEntry()
	// TODO leaky abstraction. Maybe better to support
	// an interface method for detecting possible values
	switch f := mv.response.Entries[row][col].(type) {
	case types.Enum:
		mv.selectOptions = f.Values()
		mv.switchMode(MainViewModeSelectBox)
	case types.ForeignKey:
		ftable := mv.OSM.GetDB().Tables[f.Table]
		// TODO not cache mapping
		fresp, err := ftable.Query(types.QueryParams{
			OrderBy: ftable.Columns[ftable.Primary()],
		})
		if err != nil {
			return err
		}
		values := []string{}
		for _, frow := range fresp.Entries {
			values = append(values, frow[ftable.Primary()].Format(""))
		}
		mv.selectOptions = values
		mv.switchMode(MainViewModeSelectBox)
	default:
		mv.promptText = mv.TableView.Values[row][col]
		mv.switchMode(MainViewModeEdit)
	}
	return nil
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

	if mv.Mode == MainViewModeSearch {
		err = mv.handleSearchInput(v, key, ch, mod)
		return
	}

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
		err = mv.triggerEdit()
	case gocui.KeyEsc:
		mv.TableView.SelectNone()
	case gocui.KeyPgdn:
		next := mv.nextPageStart()
		if next >= mv.response.Total {
			return
		}
		mv.Params.Offset = next
		err = mv.updateTableViewContents()
	case gocui.KeyPgup:
		mv.Params.Offset = mv.prevPageStart()
		err = mv.updateTableViewContents()
	}

	if int(ch) == 0 {
		return
	}
	switch ch {
	case 'a':
		mv.TableView.SelectColumn()
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
	case 'g', 'u':
		tables := map[string]*types.Table{}
		for tableName, table := range mv.OSM.GetDB().Tables {
			if (tableName == mv.tableName) == (ch == 'u') {
				tables[tableName] = table
			}
		}
		err = mv.goToSelectedValue(tables)
	case 'G', 'U':
		tables := map[string]*types.Table{}
		for tableName, table := range mv.OSM.GetDB().Tables {
			if (tableName == mv.tableName) == (ch == 'U') {
				tables[tableName] = table
			}
		}
		err = mv.goFromSelectedValue(tables)
	case 'f', 'F':
		row, col := mv.SelectedEntry()
		filterTarget := mv.response.Entries[row][col].Format("")
		mv.Params.Filters = append(mv.Params.Filters, &EqualFilter{
			Field:     mv.Table.Columns[col],
			Col:       col,
			Formatted: filterTarget,
			Not:       ch == 'F',
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
	case '?':
		mv.searchAll = true
		mv.Params.Filters = append(mv.Params.Filters, &ContainsFilter{
			Field:     "Any field",
			Col:       -1,
			Formatted: "",
		})
		mv.switchMode(MainViewModeSearch)
	case '/':
		mv.searchAll = false
		mv.Params.Filters = append(mv.Params.Filters, &ContainsFilter{
			Field:     mv.Table.Columns[mv.TableView.Selections.Primary.Column],
			Col:       mv.TableView.Selections.Primary.Column,
			Formatted: "",
		})
		mv.switchMode(MainViewModeSearch)
	case 'o':
		_, col := mv.SelectedEntry()
		mv.Params.OrderBy = mv.columns[col]
		mv.Params.Dec = false
		err = mv.updateTableViewContents()
	case 'O':
		_, col := mv.SelectedEntry()
		mv.Params.OrderBy = mv.columns[col]
		mv.Params.Dec = true
		err = mv.updateTableViewContents()
	case 'p':
		mv.Params.OrderBy = mv.columns[mv.Table.Primary()]
		mv.Params.Dec = false
		err = mv.updateTableViewContents()
	case 'P':
		mv.Params.OrderBy = mv.columns[mv.Table.Primary()]
		mv.Params.Dec = true
		err = mv.updateTableViewContents()
	case 'i':
		err = mv.incrementSelected(1)
	case 'I':
		err = mv.incrementSelected(-1)
	case 's':
		err = mv.saveContents()
	case 'n':
		err = mv.newEntry(true)
	case 'N':
		err = mv.newEntry(false)
	case 'w':
		err = mv.openCellInWindow()
	case 'X':
		err = mv.saveSilent()
		if err != nil {
			return
		}
		err = mv.switchView(true)
	case 'x':
		err = mv.saveSilent()
		if err != nil {
			return
		}
		err = mv.switchView(false)
	case 'y':
		err = mv.copyValue()
	case 'Y':
		err = mv.pasteValue()
	default:
		err = mv.runMacro(ch)
	}
}

func (mv *MainView) nextPageStart() uint {
	return types.IntMin(mv.Params.Offset+mv.Params.Limit, mv.Params.Offset+uint(len(mv.response.Entries)))
}

func (mv *MainView) prevPageStart() uint {
	if mv.Params.Offset < mv.Params.Limit {
		return 0
	}
	return mv.Params.Offset - mv.Params.Limit
}

func (mv *MainView) headerContents() []byte {
	// Actual params are 0-indexed but displayed as 1-indexed
	l1 := fmt.Sprintf("Table: %s\t\t\t Entries %d - %d of %d (%d total)",
		mv.tableName, mv.Params.Offset+1, mv.nextPageStart(), mv.response.Total, len(mv.Table.Entries))
	subqs := make([]string, len(mv.Params.Filters))
	for i, filter := range mv.Params.Filters {
		subqs[i] = filter.Description()
	}
	l2 := fmt.Sprintf("Query: %s", strings.Join(subqs, ", "))
	return []byte(fmt.Sprintf("%s\n%s", l1, l2))
}

func (mv *MainView) updateTableViewContents() error {
	mv.TableView.Values = [][]string{}
	// NOTE putting this here to support swapping columns later
	header := []string{}
	for _, i := range mv.colix {
		col := mv.columns[i]
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

	response, err := mv.Table.Query(mv.Params)
	if err != nil {
		return err
	}
	mv.response = response
	for _, row := range mv.response.Entries {
		formatted := []string{}
		for _, i := range mv.colix {
			entry := row[i]
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
		err = mv.updateEntryValue(contents)
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
				err = fmt.Errorf("create-new-entry takes at least 1 arg")
				return
			}
			newPK := strings.Join(parts[1:], " ")
			fields := map[string]string{}
			for _, filter := range mv.Params.Filters {
				col, formatted := filter.Example()
				if col == -1 {
					// TODO should unapply all filters here
					continue
				}
				fields[mv.Table.Columns[col]] = formatted
			}

			_, err = mv.dbms.WriteEntry(context.Background(), &jqlpb.WriteEntryRequest{
				Table:  mv.tableName,
				Pk:     newPK,
				Fields: fields,
			})
			if err != nil {
				return
			}
			err = mv.updateTableViewContents()
			return
		default:
			err = fmt.Errorf("unknown command: %s", contents)
		}
	}
}

func (mv *MainView) goToSelectedValue(tables map[string]*types.Table) error {
	row, col := mv.SelectedEntry()
	// TODO leaky abstraction. Maybe better to support
	// an interface method for detecting foreigns
	var table string
	var keys []string
	// Look for the first column, starting at the primary
	// selection that is a foreign key
loop:
	for {
		switch f := mv.response.Entries[row][col].(type) {
		case types.ForeignKey:
			table = f.Table
			keys = []string{f.Key}
			if tables[table] != nil {
				break loop
			}
		case types.ForeignList:
			table = f.Table
			keys = f.Keys
			if tables[table] != nil {
				break loop
			}
		}
		col = (col + 1) % len(mv.response.Entries[row])
		if col == mv.TableView.Selections.Primary.Column {
			return fmt.Errorf("no foreign key found in entry")
		}
	}
	err := mv.loadTable(table)
	if err != nil {
		return err
	}
	primary := mv.OSM.GetDB().Tables[table].Primary()
	var filter types.Filter
	if len(keys) == 1 {
		filter = &EqualFilter{
			Field:     mv.Table.Columns[primary],
			Col:       primary,
			Formatted: keys[0],
		}
	} else {
		filter = &InFilter{
			Field:     mv.Table.Columns[primary],
			Col:       primary,
			Formatted: slice2map(keys),
		}
	}
	mv.Params.Filters = []types.Filter{filter}
	return mv.updateTableViewContents()
}

func (mv *MainView) goFromSelectedValue(tables map[string]*types.Table) error {
	row, _ := mv.SelectedEntry()
	selected := mv.response.Entries[row][mv.Table.Primary()]
	for name, table := range tables {
		col := table.HasForeign(mv.tableName, selected.Format(""))
		if col == -1 {
			continue
		}
		secondary := mv.TableView.Selections.Secondary

		var filters []types.Filter

		if len(secondary) == 0 {
			filters = []types.Filter{
				&ContainsFilter{
					Field:     fmt.Sprintf("%s.%s", table.Columns[col], mv.Table.Columns[mv.Table.Primary()]),
					Col:       col,
					Formatted: selected.Format(""),
					Exact:     true,
				},
			}
		} else {
			// NOTE multiple selections will not work for foreign lists
			// A better solution that also would remove some hackiness in the ContainsFilter would be
			// to add a method on Entries to get their subentries
			selections := map[string]bool{selected.Format(""): true}
			for coordinate, _ := range secondary {
				selections[mv.response.Entries[coordinate.Row][mv.Table.Primary()].Format("")] = true
			}
			filters = []types.Filter{
				&InFilter{
					Field:     fmt.Sprintf("%s.%s", table.Columns[col], mv.Table.Columns[mv.Table.Primary()]),
					Col:       col,
					Formatted: selections,
				},
			}
		}
		err := mv.loadTable(name)
		if err != nil {
			return err
		}
		mv.Params.Filters = filters
		return mv.updateTableViewContents()
	}
	return fmt.Errorf("no tables found with corresponding foreign key: %s", selected)
}

func (mv *MainView) incrementSelected(amt int) error {
	row, col := mv.SelectedEntry()
	entry := mv.response.Entries[row][col]
	key := mv.response.Entries[row][mv.Table.Primary()].Format("")
	// TODO leaky abstraction
	switch typed := entry.(type) {
	case types.ForeignKey:
		ftable := mv.OSM.GetDB().Tables[typed.Table]
		// TODO not cache mapping
		fresp, err := ftable.Query(types.QueryParams{
			OrderBy: ftable.Columns[ftable.Primary()],
		})
		if err != nil {
			return err
		}
		index := map[string]int{}
		for i, fentry := range fresp.Entries {
			index[fentry[ftable.Primary()].Format("")] = i
		}
		next := (index[entry.Format("")] + 1) % len(fresp.Entries)
		err = mv.Table.Update(key, mv.Table.Columns[col], fresp.Entries[next][ftable.Primary()].Format(""))
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
	row, col := mv.SelectedEntry()
	entry := mv.response.Entries[row][col]
	cmd := exec.Command("txtopen", entry.Format(""))
	return cmd.Run()
}

func (mv *MainView) deleteSelectedRow() error {
	row, _ := mv.SelectedEntry()
	key := mv.response.Entries[row][mv.Table.Primary()].Format("")
	return mv.Table.Delete(key)
}

func (mv *MainView) nextAvailablePrimaryFromPattern(key string) (string, error) {
	ordinalFinder := regexp.MustCompile("[\\[\\(][0-9]+[\\]\\)]")
	ordinal := 0
	newKey := ""
	existing := ordinalFinder.FindAllStringSubmatchIndex(key, -1)
	var err error
	if len(existing) > 0 {
		// go with the last pattern
		used := existing[len(existing)-1]
		ordinal, err = strconv.Atoi(key[used[0]+1 : used[1]-1])
		if err != nil {
			return "", fmt.Errorf("Failed to increment ordinal: %s", err)
		}
	}
	for {
		ordinal += 1
		if len(existing) == 0 {
			newKey = fmt.Sprintf("%s (%d)", key, ordinal)
		} else {
			used := existing[len(existing)-1]
			padding := strconv.Itoa((used[1] - 1) - (used[0] + 1))
			newKey = fmt.Sprintf("%s%0"+padding+"d%s", key[:used[0]+1], ordinal, key[used[1]-1:])
		}
		if _, ok := mv.Table.Entries[newKey]; !ok {
			break
		}
	}
	return newKey, nil
}

func (mv *MainView) duplicateSelectedRow() error {
	row, _ := mv.SelectedEntry()
	primaryIndex := mv.Table.Primary()
	old := mv.response.Entries[row]
	key := old[primaryIndex].Format("")
	newKey, err := mv.nextAvailablePrimaryFromPattern(key)
	if err != nil {
		return err
	}
	err = mv.Table.Insert(newKey)
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

func (mv *MainView) copyValue() error {
	row, col := mv.SelectedEntry()
	value := mv.response.Entries[row][col].Format("")
	path, err := exec.LookPath("txtcopy")
	if err != nil {
		return err
	}
	cmd := exec.Command(path)
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	_, err = pipe.Write([]byte(value))
	if err != nil {
		return err
	}
	err = pipe.Close()
	if err != nil {
		return err
	}
	return cmd.Wait()
}

func (mv *MainView) updateEntryValue(contents string) error {
	row, column := mv.SelectedEntry()
	primary := mv.Table.Primary()
	key := mv.response.Entries[row][primary].Format("")
	err := mv.Table.Update(key, mv.Table.Columns[column], contents)
	if err != nil {
		return err
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) pasteValue() error {
	path, err := exec.LookPath("txtpaste")
	if err != nil {
		return err
	}
	cmd := exec.Command(path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return mv.updateEntryValue(strings.TrimSpace(string(out)))
}

func (mv *MainView) editWorkspace() error {
	row, col := mv.SelectedEntry()
	val := mv.response.Entries[row][col].Format("")
	ws := os.Getenv("JQL_WORKSPACE")
	dir := filepath.Join(ws, val)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}
	args := strings.Split(os.Getenv("JQL_EDITOR"), " ")
	args = append(args, filepath.Join(dir, "README.md"))
	cmd := exec.Command(args[0], args[1:]...)
	// command should run async in the background
	return cmd.Start()
}

// switchView changes to another tool for viewing the current jql db
func (mv *MainView) switchView(reverse bool) error {
	var tool string
	if reverse {
		tool = os.Getenv("JQL_REVERSE_TOOL")
		if tool == "" {
			tool = "feed"
		}
	} else {
		tool = os.Getenv("JQL_FORWARD_TOOL")
		if tool == "" {
			tool = "execute"
		}
	}

	binary, err := exec.LookPath(tool)
	if err != nil {
		return err
	}

	args := []string{tool, mv.path}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
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
	return nil
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
	return nil
}

func (mv *MainView) runMacro(ch rune) error {
	macros := mv.OSM.GetDB().Tables[MacroTable]

	entry, ok := macros.Entries[string(ch)]
	if !ok {
		return fmt.Errorf("No macro found for: '%s'", string(ch))
	}
	loc := strings.Split(entry[macros.IndexOfField(MacroLocationCol)].Format(""), " ")
	reloadIndex := macros.IndexOfField("Reload")
	isReload := reloadIndex != -1 && entry[reloadIndex].Format("") == "yes"
	var stdout, stderr bytes.Buffer
	snapshot, err := mv.OSM.GetSnapshot(mv.OSM.GetDB())
	if err != nil {
		return fmt.Errorf("Could not create snapshot: %s", err)
	}
	paramsNoLimit := types.QueryParams{
		Filters: mv.Params.Filters,
		OrderBy: mv.Params.OrderBy,
	}
	response, err := mv.Table.Query(paramsNoLimit)
	if err != nil {
		return err
	}
	pks := []string{}
	for _, entry := range response.Entries {
		pks = append(pks, entry[mv.Table.Primary()].Format(""))
	}
	row, _ := mv.SelectedEntry()
	primarySelection := mv.response.Entries[row][mv.Table.Primary()]

	input := MacroInterface{
		Snapshot: snapshot,
		CurrentView: MacroCurrentView{
			Table:            mv.tableName,
			PKs:              pks,
			PrimarySelection: primarySelection.Format(""),
		},
	}
	inputEncoded, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("Could not marshal input: %s", err)
	}
	cmd := exec.Command(loc[0], loc[1:]...)
	cmd.Stdin = bytes.NewBuffer(inputEncoded)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		// TODO log stderr
		writeErr := ioutil.WriteFile("/tmp/error.log", stderr.Bytes(), os.ModePerm)
		if writeErr != nil {
			return fmt.Errorf("Could not run macro or store stderr: %s", err)
		}
		return fmt.Errorf("Could not run macro: %s -- error at /tmp/error.log", err)
	}
	var newDB []byte
	var output MacroInterface

	// TODO change to three valued "Output" field: file, stdout, none
	if isReload {
		newDB, err = ioutil.ReadFile(mv.path)
		if err != nil {
			return fmt.Errorf("Could not reload db: %s", err)
		}
	} else {
		err = json.Unmarshal(stdout.Bytes(), &output)
		if err != nil {
			return fmt.Errorf("Could not unmarshal macro output: %s", err)
		}
		newDB = []byte(output.Snapshot)
	}
	err = mv.OSM.LoadSnapshot(bytes.NewBuffer(newDB))
	if err != nil {
		return fmt.Errorf("Could not load database from macro: %s", err)
	}
	tableSwitch := mv.tableName != output.CurrentView.Table
	params := mv.Params
	err = mv.loadTable(output.CurrentView.Table)
	if err != nil {
		return fmt.Errorf("Could not load table after macro: %s", err)
	}
	if !tableSwitch {
		mv.Params = params
	}
	filterField := output.CurrentView.Filter.Field
	if filterField != "" {
		// The macro is updating our table query with a basic filter
		// For now this only supports a single equal filter
		// TODO figure out why the Query in the header doesn't update until after another button push
		mv.Params.Filters = []types.Filter{
			&EqualFilter{
				Field:     filterField,
				Col:       mv.Table.IndexOfField(filterField),
				Formatted: output.CurrentView.Filter.Formatted,
			},
		}
	}
	orderBy := output.CurrentView.OrderBy
	if orderBy != "" {
		mv.Params.OrderBy = orderBy
		mv.Params.Dec = output.CurrentView.OrderDec
	}
	err = mv.updateTableViewContents()
	if err != nil {
		return fmt.Errorf("Could not update table view after macro: %s", err)
	}
	return fmt.Errorf("Ran macro %s", loc)
}

func (mv *MainView) SelectedEntry() (int, int) {
	row, col := mv.TableView.PrimarySelection()
	return row, mv.colix[col]
}

func (mv *MainView) GoToPrimaryKey(pk string) error {
	primary := mv.Table.Primary()
	field := mv.Table.Columns[primary]
	mv.Params.Filters = []types.Filter{
		&EqualFilter{
			Field:     field,
			Col:       primary,
			Formatted: pk,
		},
	}
	return mv.updateTableViewContents()
}
