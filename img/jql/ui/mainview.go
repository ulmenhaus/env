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
	path string
	dbms api.JQL_DBMS

	OSM     *osm.ObjectStoreMapper
	Table   *types.Table
	Request jqlpb.ListRowsRequest
	columns []string
	colix   []int
	// TODO map[string]types.Entry and []types.Entry could both
	// be higher-level types (e.g. VerboseRow and Row)
	response    *types.Response
	tmpResponse *jqlpb.ListRowsResponse

	TableView *TableView
	Mode      MainViewMode

	switching     bool // on when transitioning modes has not yet been acknowleged by Layout
	alert         string
	promptText    string
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

func (mv *MainView) maxWidths(t *types.Table, tName string) ([]int, error) {
	// TODO can consolidate this into a single request with the main request
	// once that also uses the dbms interface
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: tName,
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	max := make([]int, len(resp.Columns))
	for i, colMeta := range resp.Columns {
		max[i] = int(colMeta.MaxLength)
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
	mv.Request.Table = tName
	columns := []string{}
	colix := []int{}
	widths := []int{}
	max, err := mv.maxWidths(table, tName)
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
	mv.Request.OrderBy = mv.columns[0]
	mv.Request.Conditions = []*jqlpb.Condition{{}}
	mv.Request.Offset = uint32(0)
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
	maxRows := uint32(maxY - 8)
	if mv.Request.Limit != maxRows {
		mv.Request.Limit = maxRows
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
	primarySelection := &jqlpb.Entry{}
	if row < len(mv.tmpResponse.Rows) {
		primarySelection = mv.tmpResponse.Rows[row].Entries[mv.Table.Primary()]
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
		for _, filter := range mv.Request.Conditions[0].Requires {
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
	mv.Request.Offset = uint32(0)
	// When switching into search mode, the last filter added is the working
	// search filter

	field := mv.Table.Columns[mv.TableView.Selections.Primary.Column]
	if mv.searchAll {
		field = "Any field"
	}
	mv.Request.Conditions[0].Requires[len(mv.Request.Conditions[0].Requires)-1] = &jqlpb.Filter{
		Column: field,
		Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: mv.searchText}},
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
		if next >= uint(mv.tmpResponse.Total) {
			return
		}
		mv.Request.Offset = uint32(next)
		err = mv.updateTableViewContents()
	case gocui.KeyPgup:
		mv.Request.Offset = uint32(mv.prevPageStart())
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
			if (tableName == mv.Request.Table) == (ch == 'u') {
				tables[tableName] = table
			}
		}
		err = mv.goToSelectedValue(tables)
	case 'G', 'U':
		tables := map[string]*types.Table{}
		for tableName, table := range mv.OSM.GetDB().Tables {
			if (tableName == mv.Request.Table) == (ch == 'U') {
				tables[tableName] = table
			}
		}
		err = mv.goFromSelectedValue(tables)
	case 'f', 'F':
		row, col := mv.SelectedEntry()
		filterTarget := mv.tmpResponse.Rows[row].Entries[col].Formatted
		mv.Request.Conditions[0].Requires = append(mv.Request.Conditions[0].Requires, &jqlpb.Filter{
			Negated: ch == 'F',
			Column:  mv.Table.Columns[col],
			Match:   &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: filterTarget}},
		})
		err = mv.updateTableViewContents()
	case 'q':
		if len(mv.Request.Conditions[0].Requires) > 0 {
			mv.Request.Conditions[0].Requires = mv.Request.Conditions[0].Requires[:len(mv.Request.Conditions[0].Requires)-1]
		}
		err = mv.updateTableViewContents()
	case 'Q':
		mv.Request.Conditions = []*jqlpb.Condition{{}}
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
		mv.Request.Conditions[0].Requires = append(mv.Request.Conditions[0].Requires, &jqlpb.Filter{
			Column: "",
			Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: ""}},
		})
		mv.switchMode(MainViewModeSearch)
	case '/':
		mv.searchAll = false
		mv.Request.Conditions[0].Requires = append(mv.Request.Conditions[0].Requires, &jqlpb.Filter{
			Column: mv.Table.Columns[mv.TableView.Selections.Primary.Column],
			Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: ""}},
		})
		mv.switchMode(MainViewModeSearch)
	case 'o':
		_, col := mv.SelectedEntry()
		mv.Request.OrderBy = mv.columns[col]
		mv.Request.Dec = false
		err = mv.updateTableViewContents()
	case 'O':
		_, col := mv.SelectedEntry()
		mv.Request.OrderBy = mv.columns[col]
		mv.Request.Dec = true
		err = mv.updateTableViewContents()
	case 'p':
		mv.Request.OrderBy = mv.columns[mv.Table.Primary()]
		mv.Request.Dec = false
		err = mv.updateTableViewContents()
	case 'P':
		mv.Request.OrderBy = mv.columns[mv.Table.Primary()]
		mv.Request.Dec = true
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
	return types.IntMin(uint(mv.Request.Offset+mv.Request.Limit), uint(mv.Request.Offset)+uint(len(mv.tmpResponse.Rows)))
}

func (mv *MainView) prevPageStart() uint {
	if mv.Request.Offset < mv.Request.Limit {
		return 0
	}
	return uint(mv.Request.Offset - mv.Request.Limit)
}

func (mv *MainView) headerContents() []byte {
	// Actual params are 0-indexed but displayed as 1-indexed
	l1 := fmt.Sprintf("Table: %s\t\t\t Entries %d - %d of %d (%d total)",
		mv.Request.Table, mv.Request.Offset+1, mv.nextPageStart(), mv.tmpResponse.Total, len(mv.Table.Entries))
	subqs := make([]string, len(mv.Request.Conditions[0].Requires))
	for i, filter := range mv.Request.Conditions[0].Requires {
		subqs[i] = api.Description(filter)
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
		if mv.Request.OrderBy == col {
			if mv.Request.Dec {
				col += " ^"
			} else {
				col += " v"
			}
		}
		header = append(header, col)
	}
	mv.TableView.Header = header

	response, err := mv.dbms.ListRows(ctx, &mv.Request)
	if err != nil {
		return err
	}
	mv.tmpResponse = response
	for _, row := range mv.tmpResponse.Rows {
		formatted := []string{}
		for _, i := range mv.colix {
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
			for _, filter := range mv.Request.Conditions[0].Requires {
				col, formatted := api.Example(filter)
				if col == -1 {
					// TODO should unapply all filters here
					continue
				}
				fields[mv.Table.Columns[col]] = formatted
			}

			_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
				Table:  mv.Request.Table,
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
	var filter *jqlpb.Filter
	if len(keys) == 1 {
		filter = &jqlpb.Filter{
			Column: mv.Table.Columns[primary],
			Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: keys[0]}},
		}
	} else {
		filter = &jqlpb.Filter{
			Column: mv.Table.Columns[primary],
			Match:  &jqlpb.Filter_InMatch{&jqlpb.InMatch{Values: keys}},
		}
	}
	mv.Request.Conditions[0].Requires = []*jqlpb.Filter{filter}
	return mv.updateTableViewContents()
}

func (mv *MainView) goFromSelectedValue(tables map[string]*types.Table) error {
	row, _ := mv.SelectedEntry()
	selected := mv.response.Entries[row][mv.Table.Primary()]
	for name, table := range tables {
		col := table.HasForeign(mv.Request.Table, selected.Format(""))
		if col == -1 {
			continue
		}
		secondary := mv.TableView.Selections.Secondary

		var filters []*jqlpb.Filter

		if len(secondary) == 0 {
			filters = []*jqlpb.Filter{
				{
					Column: mv.columns[col],
					Match:  &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: selected.Format(""), Exact: true}},
				},
			}
		} else {
			// NOTE multiple selections will not work for foreign lists
			// A better solution that also would remove some hackiness in the ContainsFilter would be
			// to add a method on Entries to get their subentries
			selections := []string{selected.Format("")}
			for coordinate, _ := range secondary {
				selections = append(selections, mv.response.Entries[coordinate.Row][mv.Table.Primary()].Format(""))
			}
			filters = []*jqlpb.Filter{
				{
					Column: mv.columns[col],
					Match:  &jqlpb.Filter_InMatch{InMatch: &jqlpb.InMatch{Values: selections}},
				},
			}
		}
		err := mv.loadTable(name)
		if err != nil {
			return err
		}
		mv.Request.Conditions[0].Requires = filters
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
	fields := map[string]string{}
	for i, oldValue := range old {
		if i == primaryIndex {
			continue
		}
		fields[mv.Table.Columns[i]] = oldValue.Format("")
	}
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:  mv.Request.Table,
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
	_, err := mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table:      mv.Request.Table,
		Pk:         mv.response.Entries[row][mv.Table.Primary()].Format(""),
		Fields:     map[string]string{mv.Table.Columns[column]: contents},
		UpdateOnly: true,
	})
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
	snapshot, err := mv.OSM.GetSnapshot(mv.OSM.GetDB())
	if err != nil {
		return fmt.Errorf("Could not create snapshot: %s", err)
	}
	requestNoLimit := &jqlpb.ListRowsRequest{
		Conditions: mv.Request.Conditions,
		OrderBy:    mv.Request.OrderBy,
	}
	response, err := mv.dbms.ListRows(ctx, requestNoLimit)
	if err != nil {
		return err
	}
	pks := []string{}
	for _, row := range response.Rows {
		pks = append(pks, row.Entries[mv.Table.Primary()].Formatted)
	}
	row, _ := mv.SelectedEntry()
	primarySelection := mv.response.Entries[row][mv.Table.Primary()]

	input := MacroInterface{
		Snapshot: snapshot,
		CurrentView: MacroCurrentView{
			Table:            mv.Request.Table,
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
	tableSwitch := mv.Request.Table != output.CurrentView.Table
	request := mv.Request
	err = mv.loadTable(output.CurrentView.Table)
	if err != nil {
		return fmt.Errorf("Could not load table after macro: %s", err)
	}
	if !tableSwitch {
		mv.Request = request
	}
	filterField := output.CurrentView.Filter.Field
	if filterField != "" {
		// The macro is updating our table query with a basic filter
		// For now this only supports a single equal filter
		// TODO figure out why the Query in the header doesn't update until after another button push
		mv.Request.Conditions = []*jqlpb.Condition{
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
		mv.Request.OrderBy = orderBy
		mv.Request.Dec = output.CurrentView.OrderDec
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
	mv.Request.Conditions = []*jqlpb.Condition{
		{
			Requires: []*jqlpb.Filter{
				{
					Column: mv.columns[mv.Table.Primary()],
					Match:  &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: pk}},
				},
			},
		},
	}
	return mv.updateTableViewContents()
}
