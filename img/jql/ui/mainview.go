package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

var (
	ctx = context.Background()
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
	dbms api.JQL_DBMS

	TableView *TableView
	Mode      MainViewMode

	request  jqlpb.ListRowsRequest
	response *jqlpb.ListRowsResponse

	switching     bool // on when transitioning modes has not yet been acknowleged by Layout
	alert         string
	promptText    string
	searchText    string
	searchAll     bool // indicates if we search all fields or just this one
	selectOptions []string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(dbms api.JQL_DBMS, start string) (*MainView, error) {
	mv := &MainView{
		dbms: dbms,
	}
	return mv, mv.loadTable(start)
}

// loadTable displays the named table in the main table view
func (mv *MainView) loadTable(t string) error {
	mv.request = jqlpb.ListRowsRequest{
		Table:      t,
		Conditions: []*jqlpb.Condition{{}},
	}
	return mv.updateTableViewContents(true)
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
	maxRows := uint32(maxY - 8)
	if mv.request.Limit != maxRows {
		mv.request.Limit = maxRows
		if err := mv.updateTableViewContents(true); err != nil {
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
	primarySelection := &jqlpb.Entry{}
	if row < len(mv.response.Rows) {
		primarySelection = mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)]
	}
	location.Write([]byte(fmt.Sprintf("    L%d C%d           %s", row, col, primarySelection.Formatted)))
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
		for _, filter := range mv.request.Conditions[0].Requires {
			suggestion, yes := api.PrimarySuggestion(filter)
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
	err := mv.Save()
	if err != nil {
		return err
	}
	return fmt.Errorf("Wrote database")
}

func (mv *MainView) Save() error {
	_, err := mv.dbms.Persist(ctx, &jqlpb.PersistRequest{})
	return err
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
	mv.request.Offset = uint32(0)
	// When switching into search mode, the last filter added is the working
	// search filter

	field := mv.response.Columns[mv.TableView.Selections.Primary.Column].Name
	if mv.searchAll {
		field = "Any field"
	}
	mv.request.Conditions[0].Requires[len(mv.request.Conditions[0].Requires)-1] = &jqlpb.Filter{
		Column: field,
		Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: mv.searchText}},
	}
	return mv.updateTableViewContents(true)
}

func (mv *MainView) triggerEdit() error {
	row, col := mv.SelectedEntry()
	meta := mv.response.Columns[col]
	switch meta.Type {
	case jqlpb.EntryType_ENUM:
		mv.selectOptions = meta.Values
		mv.switchMode(MainViewModeSelectBox)
	case jqlpb.EntryType_FOREIGN:
		resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
			Table: meta.ForeignTable,
		})
		if err != nil {
			return err
		}
		values := []string{}
		primary := api.GetPrimary(resp.Columns)
		for _, row := range resp.Rows {
			values = append(values, row.Entries[primary].Formatted)
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
		// NOTE this mapping will no longer work since Esc is used to switch between tools
		// but we don't really use multi-select for anything. If we want to support it again
		// we can probably remap `q` to first try a multi-select and then remove filters
		// if there's nothing to deselect.
		mv.TableView.SelectNone()
	case gocui.KeyPgdn:
		next := mv.nextPageStart()
		if next >= uint(mv.response.Total) {
			return
		}
		mv.request.Offset = uint32(next)
		err = mv.updateTableViewContents(true)
	case gocui.KeyPgup:
		mv.request.Offset = uint32(mv.prevPageStart())
		err = mv.updateTableViewContents(true)
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
		var resp *jqlpb.ListTablesResponse
		resp, err = mv.dbms.ListTables(ctx, &jqlpb.ListTablesRequest{})
		if err != nil {
			return
		}
		tables := map[string]*jqlpb.TableMeta{}
		for _, table := range resp.Tables {
			if (table.Name == mv.request.Table) == (ch == 'u') {
				tables[table.Name] = table
			}
		}
		err = mv.goToSelectedValue(tables)
	case 'G', 'U':
		var resp *jqlpb.ListTablesResponse
		resp, err = mv.dbms.ListTables(ctx, &jqlpb.ListTablesRequest{})
		if err != nil {
			return
		}
		var tables []*jqlpb.TableMeta
		for _, table := range resp.Tables {
			if (table.Name == mv.request.Table) == (ch == 'U') {
				tables = append(tables, table)
			}
		}
		err = mv.goFromSelectedValue(tables)
	case 'f', 'F':
		row, col := mv.SelectedEntry()
		filterTarget := mv.response.Rows[row].Entries[col].Formatted
		mv.request.Conditions[0].Requires = append(mv.request.Conditions[0].Requires, &jqlpb.Filter{
			Negated: ch == 'F',
			Column:  mv.response.Columns[col].Name,
			Match:   &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: filterTarget}},
		})
		err = mv.updateTableViewContents(true)
	case 'q':
		if len(mv.request.Conditions[0].Requires) > 0 {
			mv.request.Conditions[0].Requires = mv.request.Conditions[0].Requires[:len(mv.request.Conditions[0].Requires)-1]
		}
		err = mv.updateTableViewContents(true)
	case 'Q':
		mv.request.Conditions = []*jqlpb.Condition{{}}
		err = mv.updateTableViewContents(true)
	case 'd':
		err = mv.deleteSelectedRow()
		if err != nil {
			return
		}
		err = mv.updateTableViewContents(false)
	case 'D':
		err = mv.duplicateSelectedRow()
		if err != nil {
			return
		}
		err = mv.updateTableViewContents(false)
	case '\'':
		mv.switchMode(MainViewModePrompt)
		mv.promptText = "switch-table "
	case ':':
		mv.switchMode(MainViewModePrompt)
	case '?':
		mv.searchAll = true
		mv.request.Conditions[0].Requires = append(mv.request.Conditions[0].Requires, &jqlpb.Filter{
			Column: "",
			Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: ""}},
		})
		mv.switchMode(MainViewModeSearch)
	case '/':
		mv.searchAll = false
		mv.request.Conditions[0].Requires = append(mv.request.Conditions[0].Requires, &jqlpb.Filter{
			Column: mv.response.Columns[mv.TableView.Selections.Primary.Column].Name,
			Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: ""}},
		})
		mv.switchMode(MainViewModeSearch)
	case 'o':
		_, col := mv.SelectedEntry()
		mv.request.OrderBy = mv.response.Columns[col].Name
		mv.request.Dec = false
		err = mv.updateTableViewContents(true)
	case 'O':
		_, col := mv.SelectedEntry()
		mv.request.OrderBy = mv.response.Columns[col].Name
		mv.request.Dec = true
		err = mv.updateTableViewContents(true)
	case 'p':
		mv.request.OrderBy = mv.response.Columns[api.GetPrimary(mv.response.Columns)].Name
		mv.request.Dec = false
		err = mv.updateTableViewContents(true)
	case 'P':
		mv.request.OrderBy = mv.response.Columns[api.GetPrimary(mv.response.Columns)].Name
		mv.request.Dec = true
		err = mv.updateTableViewContents(true)
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
	case 'y':
		err = mv.copyValue()
	case 'Y':
		err = mv.pasteValue()
	default:
		err = mv.runMacro(ch)
	}
}

func (mv *MainView) nextPageStart() uint {
	return types.IntMin(uint(mv.request.Offset+mv.request.Limit), uint(mv.request.Offset)+uint(len(mv.response.Rows)))
}

func (mv *MainView) prevPageStart() uint {
	if mv.request.Offset < mv.request.Limit {
		return 0
	}
	return uint(mv.request.Offset - mv.request.Limit)
}

func (mv *MainView) headerContents() []byte {
	// Actual params are 0-indexed but displayed as 1-indexed
	l1 := fmt.Sprintf("Table: %s\t\t\t Entries %d - %d of %d (%d total)",
		mv.request.Table, mv.request.Offset+1, mv.nextPageStart(), mv.response.Total, mv.response.All)
	subqs := make([]string, len(mv.request.Conditions[0].Requires))
	for i, filter := range mv.request.Conditions[0].Requires {
		subqs[i] = api.Description(filter)
	}
	l2 := fmt.Sprintf("Query: %s", strings.Join(subqs, ", "))
	return []byte(fmt.Sprintf("%s\n%s", l1, l2))
}

func (mv *MainView) getColumnIndices() []int {
	var indices []int
	for i, col := range mv.response.Columns {
		if !strings.HasPrefix(col.Name, "_") {
			indices = append(indices, i)
		}
	}
	return indices
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sameColumns(respA, respB *jqlpb.ListRowsResponse) bool {
	colsA := respA.GetColumns()
	colsB := respB.GetColumns()
	if len(colsA) != len(colsB) {
		return false
	}
	for i := range colsA {
		if colsA[i].Name != colsB[i].Name {
			return false
		}
	}
	return true
}

func (mv *MainView) updateTableViewContents(resetCursorRow bool) error {
	response, err := mv.dbms.ListRows(ctx, &mv.request)
	if err != nil {
		return err
	}
	mv.request.Table = response.Table
	selectedCol := 0
	if sameColumns(response, mv.response) {
		// If after changing the contents, we're still looking at the
		// same columns, then keep the pointer in that column
		selectedCol = mv.TableView.Selections.Primary.Column
	}
	selectedRow := 0
	if !resetCursorRow && mv.TableView != nil {
		selectedRow = mv.TableView.Selections.Primary.Row
	}
	mv.response = response

	var header []string
	var widths []int
	for _, i := range mv.getColumnIndices() {
		col := mv.response.Columns[i]
		name := col.Name
		if mv.request.OrderBy == name {
			if mv.request.Dec {
				name += " ^"
			} else {
				name += " v"
			}
		}
		header = append(header, name)
		widths = append(widths, minInt(int(col.MaxLength), 40))
	}
	mv.TableView = &TableView{
		Header: header,
		Values: [][]string{},
		Widths: widths,
		Selections: SelectionSet{
			Primary: Coordinate{
				Column: selectedCol,
				Row:    selectedRow,
			},
			Secondary: make(map[Coordinate]bool),
			Tertiary:  make(map[Coordinate]bool),
		},
	}

	// NOTE putting this here to support swapping columns later
	for _, row := range mv.response.Rows {
		formatted := []string{}
		for _, i := range mv.getColumnIndices() {
			entry := row.Entries[i]
			// TODO extract actual formatting
			formatted = append(formatted, entry.Formatted)
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
			for _, filter := range mv.request.Conditions[0].Requires {
				col, formatted := api.Example(mv.response.Columns, filter)
				if col == -1 {
					// TODO should unapply all filters here
					continue
				}
				fields[mv.response.Columns[col].Name] = formatted
			}

			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				Table:  mv.request.Table,
				Pk:     newPK,
				Fields: fields,
			})
			if err != nil {
				return
			}
			err = mv.updateTableViewContents(true)
			return
		default:
			err = fmt.Errorf("unknown command: %s", contents)
		}
	}
}

func (mv *MainView) goToSelectedValue(tables map[string]*jqlpb.TableMeta) error {
	row, col := mv.SelectedEntry()
	var table string
	var keys []string
	// Look for the first column, starting at the primary
	// selection that is a foreign key
loop:
	for {
		meta := mv.response.Columns[col]
		switch meta.Type {
		case jqlpb.EntryType_FOREIGN:
			table = meta.ForeignTable
			keys = []string{mv.response.Rows[row].Entries[col].Formatted}
			if tables[table] != nil {
				break loop
			}
		case jqlpb.EntryType_FOREIGNS:
			table = meta.ForeignTable
			keys = strings.Split(mv.response.Rows[row].Entries[col].Formatted, "\n")
			if tables[table] != nil {
				break loop
			}
		}
		col = (col + 1) % len(mv.response.Columns)
		if col == mv.TableView.Selections.Primary.Column {
			return fmt.Errorf("no foreign key found in entry")
		}
	}
	err := mv.loadTable(table)
	if err != nil {
		return err
	}
	primary := api.GetPrimary(mv.response.Columns)
	var filter *jqlpb.Filter
	if len(keys) == 1 {
		filter = &jqlpb.Filter{
			Column: mv.response.Columns[primary].Name,
			Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: keys[0]}},
		}
	} else {
		filter = &jqlpb.Filter{
			Column: mv.response.Columns[primary].Name,
			Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: keys}},
		}
	}
	mv.request.Conditions[0].Requires = []*jqlpb.Filter{filter}
	return mv.updateTableViewContents(true)
}

func (mv *MainView) goFromSelectedValue(tables []*jqlpb.TableMeta) error {
	row, _ := mv.SelectedEntry()
	selected := mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)]
	for _, table := range tables {
		cols := api.GetForeign(table.Columns, mv.request.Table)
		if len(cols) == 0 {
			continue
		}
		var conditions []*jqlpb.Condition
		// If there are multiple foreign key columns, ignore any that don't have any matching entries
		for _, col := range cols {
			conditions = []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: table.Columns[col].Name,
							Match:  &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: selected.Formatted}},
						},
					},
				},
			}
			allMatching, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
				Table:      table.Name,
				Conditions: conditions,
				Limit:      1,
			})
			if err != nil {
				return err
			}
			if len(allMatching.Rows) > 0 {
				break
			}
		}

		mv.request.Table = table.Name
		mv.request.Conditions = conditions
		mv.request.OrderBy = ""
		return mv.updateTableViewContents(true)
	}
	return fmt.Errorf("no tables found with corresponding foreign key: %s", selected)
}

func (mv *MainView) incrementSelected(amt int) error {
	row, col := mv.SelectedEntry()
	_, err := mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  mv.request.Table,
		Pk:     mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)].Formatted,
		Amount: int32(amt),
		Column: mv.response.Columns[col].Name,
	})
	if err != nil {
		return err
	}
	return mv.updateTableViewContents(false)
}

func (mv *MainView) openCellInWindow() error {
	row, col := mv.SelectedEntry()
	entry := mv.response.Rows[row].Entries[col]
	cmd := exec.Command("txtopen", entry.Formatted)
	return cmd.Run()
}

func (mv *MainView) deleteSelectedRow() error {
	row, _ := mv.SelectedEntry()
	_, err := mv.dbms.DeleteRow(ctx, &jqlpb.DeleteRowRequest{
		Table: mv.request.Table,
		Pk:    mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)].Formatted,
	})
	return err
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
		_, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
			Table: mv.request.Table,
			Pk:    newKey,
		})
		if err != nil {
			// we found a key that does not exist
			// TODO would be nice to test this is specifically a not found error
			break
		}
	}
	return newKey, nil
}

func (mv *MainView) duplicateSelectedRow() error {
	row, _ := mv.SelectedEntry()
	primaryIndex := api.GetPrimary(mv.response.Columns)
	old := mv.response.Rows[row].Entries
	key := old[primaryIndex].Formatted
	newKey, err := mv.nextAvailablePrimaryFromPattern(key)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	for i, oldValue := range old {
		if i == primaryIndex {
			continue
		}
		fields[mv.response.Columns[i].Name] = oldValue.Formatted
	}
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  mv.request.Table,
		Pk:     newKey,
		Fields: fields,
	})
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) copyValue() error {
	row, col := mv.SelectedEntry()
	value := mv.response.Rows[row].Entries[col].Formatted
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
	_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      mv.request.Table,
		Pk:         mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)].Formatted,
		Fields:     map[string]string{mv.response.Columns[column].Name: contents},
		UpdateOnly: true,
	})
	if err != nil {
		return err
	}
	return mv.updateTableViewContents(false)
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
	resp, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: MacroTable,
		Pk:    string(ch),
	})
	if err != nil {
		return fmt.Errorf("Macro not found for '%s': %s", string(ch), err)
	}
	entries := resp.GetRow().GetEntries()
	locIndex := api.IndexOfField(resp.GetColumns(), MacroLocationCol)
	loc := strings.Split(entries[locIndex].GetFormatted(), " ")
	reloadIndex := api.IndexOfField(resp.GetColumns(), "Reload")
	isReload := reloadIndex != -1 && entries[reloadIndex].GetFormatted() == "yes"
	var stdout, stderr bytes.Buffer
	snapResp, err := mv.dbms.GetSnapshot(ctx, &jqlpb.GetSnapshotRequest{})
	if err != nil {
		return fmt.Errorf("Could not create snapshot: %s", err)
	}
	snapshot := snapResp.Snapshot
	requestNoLimit := &jqlpb.ListRowsRequest{
		Table:      mv.request.Table,
		Conditions: mv.request.Conditions,
		OrderBy:    mv.request.OrderBy,
		Dec:        mv.request.Dec,
	}
	response, err := mv.dbms.ListRows(ctx, requestNoLimit)
	if err != nil {
		return err
	}
	pks := []string{}
	for _, row := range response.Rows {
		pks = append(pks, row.Entries[api.GetPrimary(response.Columns)].Formatted)
	}
	row, _ := mv.SelectedEntry()
	primarySelection := mv.response.Rows[row].Entries[api.GetPrimary(mv.response.Columns)]

	input := MacroInterface{
		Snapshot: string(snapshot),
		CurrentView: MacroCurrentView{
			Table:            mv.request.Table,
			PKs:              pks,
			PrimarySelection: primarySelection.Formatted,
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
		return fmt.Errorf("Reloaded macros no longer supported. Please change the macro.")
	} else {
		err = json.Unmarshal(stdout.Bytes(), &output)
		if err != nil {
			return fmt.Errorf("Could not unmarshal macro output: %s", err)
		}
		newDB = []byte(output.Snapshot)
	}
	_, err = mv.dbms.LoadSnapshot(ctx, &jqlpb.LoadSnapshotRequest{
		Snapshot: newDB,
	})
	if err != nil {
		return fmt.Errorf("Could not load database from macro: %s", err)
	}
	tableSwitch := mv.request.Table != output.CurrentView.Table
	request := mv.request
	err = mv.loadTable(output.CurrentView.Table)
	if err != nil {
		return fmt.Errorf("Could not load table after macro: %s", err)
	}
	if !tableSwitch {
		mv.request = request
	}
	filterField := output.CurrentView.Filter.Field
	if filterField != "" {
		// The macro is updating our table query with a basic filter
		// For now this only supports a single equal filter
		// TODO figure out why the Query in the header doesn't update until after another button push
		mv.request.Conditions = []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: filterField,
						Match:  &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: output.CurrentView.Filter.Formatted}},
					},
				},
			},
		}
	}
	orderBy := output.CurrentView.OrderBy
	if orderBy != "" {
		mv.request.OrderBy = orderBy
		mv.request.Dec = output.CurrentView.OrderDec
	}
	err = mv.updateTableViewContents(true)
	if err != nil {
		return fmt.Errorf("Could not update table view after macro: %s", err)
	}
	return fmt.Errorf("Ran macro %s", loc)
}

func (mv *MainView) SelectedEntry() (int, int) {
	row, col := mv.TableView.PrimarySelection()
	return row, mv.getColumnIndices()[col]
}

func (mv *MainView) GoToPrimaryKey(pk string) error {
	mv.request.Conditions = []*jqlpb.Condition{
		{
			Requires: []*jqlpb.Filter{
				{
					Column: mv.response.Columns[api.GetPrimary(mv.response.Columns)].Name,
					Match:  &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: pk}},
				},
			},
		},
	}
	return mv.updateTableViewContents(true)
}
