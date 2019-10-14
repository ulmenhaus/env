package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

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
	// MainViewModeSearch is for when the user is typing
	// a search query into the view
	MainViewModeSearch
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
	response *types.Response

	TableView *TableView
	Mode      MainViewMode

	switching  bool // on when transitioning modes has not yet been acknowleged by Layout
	alert      string
	promptText string
	tableName  string
	searchText string
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
	table := mv.DB.Tables[tName]
	mv.Table = table
	mv.tableName = tName
	columns := []string{}
	widths := []int{}
	max, err := mv.maxWidths(table)
	if err != nil {
		return err
	}
	for i, column := range table.Columns {
		if strings.HasPrefix(column, "_") {
			// TODO note these to skip the values as well
			continue
		}
		width := 40
		if max != nil && max[i] < width {
			width = max[i]
		}
		widths = append(widths, width)
		columns = append(columns, column)
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
	// TODO would be good to preserve params per table
	mv.Params.OrderBy = mv.columns[0]
	mv.Params.Filters = []types.Filter{}
	mv.Params.Offset = uint(0)
	return mv.updateTableViewContents()
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
	v, err := g.SetView("table", 0, 3, maxX-2, maxY-3)
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
		prompt.Write([]byte{'/'})
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
	f, err := os.OpenFile(mv.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return mv.OSM.Dump(mv.DB, f)
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
	} else {
		mv.searchText += string(ch)
		// when we start search pagination should be reset
		mv.Params.Offset = uint(0)
	}
	// TODO implement major search mode (over all columns)
	// When switching into search mode, the last filter added is the working
	// search filter
	mv.Params.Filters[len(mv.Params.Filters)-1] = &ContainsFilter{
		Field:     mv.Table.Columns[mv.TableView.Selections.Primary.Column],
		Col:       mv.TableView.Selections.Primary.Column,
		Formatted: mv.searchText,
	}
	return mv.updateTableViewContents()
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
		mv.switchMode(MainViewModeEdit)
		row, column := mv.TableView.PrimarySelection()
		mv.promptText = mv.TableView.Values[row][column]
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
	case 'g':
		err = mv.goToSelectedValue()
	case 'G':
		err = mv.goFromSelectedValue()
	case 'f', 'F':
		row, col := mv.TableView.PrimarySelection()
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
	case '/':
		mv.Params.Filters = append(mv.Params.Filters, &ContainsFilter{
			Field:     mv.Table.Columns[mv.TableView.Selections.Primary.Column],
			Col:       mv.TableView.Selections.Primary.Column,
			Formatted: "",
		})
		mv.switchMode(MainViewModeSearch)
	case 'o':
		_, col := mv.TableView.PrimarySelection()
		mv.Params.OrderBy = mv.columns[col]
		mv.Params.Dec = false
		err = mv.updateTableViewContents()
	case 'O':
		_, col := mv.TableView.PrimarySelection()
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
	case 'e':
		err = mv.editWorkspace()
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

	response, err := mv.Table.Query(mv.Params)
	if err != nil {
		return err
	}
	mv.response = response
	for _, row := range mv.response.Entries {
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
	row, col := mv.TableView.PrimarySelection()
	// TODO leaky abstraction. Maybe better to support
	// an interface method for detecting foreigns
	var foreign types.ForeignKey
	// Look for the first column, starting at the primary
	// selection that is a foreign key
	for {
		if f, ok := mv.response.Entries[row][col].(types.ForeignKey); ok {
			foreign = f
			break
		}
		col = (col + 1) % len(mv.response.Entries[row])
		if col == mv.TableView.Selections.Primary.Column {
			return fmt.Errorf("no foreign key found in entry")
		}
	}
	err := mv.loadTable(foreign.Table)
	if err != nil {
		return err
	}
	primary := mv.DB.Tables[foreign.Table].Primary()
	mv.Params.Filters = []types.Filter{
		&EqualFilter{
			Field:     mv.Table.Columns[primary],
			Col:       primary,
			Formatted: foreign.Key,
		},
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) goFromSelectedValue() error {
	row, _ := mv.TableView.PrimarySelection()
	selected := mv.response.Entries[row][mv.Table.Primary()]
	for name, table := range mv.DB.Tables {
		col := table.HasForeign(mv.tableName)
		if col == -1 {
			continue
		}
		secondary := mv.TableView.Selections.Secondary

		var filters []types.Filter

		if len(secondary) == 0 {
			filters = []types.Filter{
				&EqualFilter{
					Field:     fmt.Sprintf("%s.%s", table.Columns[col], mv.Table.Columns[mv.Table.Primary()]),
					Col:       col,
					Formatted: selected.Format(""),
				},
			}
		} else {
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
	row, col := mv.TableView.PrimarySelection()
	entry := mv.response.Entries[row][col]
	key := mv.response.Entries[row][mv.Table.Primary()].Format("")
	// TODO leaky abstraction
	switch typed := entry.(type) {
	case types.ForeignKey:
		ftable := mv.DB.Tables[typed.Table]
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
	row, col := mv.TableView.PrimarySelection()
	entry := mv.response.Entries[row][col]
	cmd := exec.Command("open", entry.Format(""))
	return cmd.Run()
}

func (mv *MainView) deleteSelectedRow() error {
	row, _ := mv.TableView.PrimarySelection()
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
	row, _ := mv.TableView.PrimarySelection()
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
	row, col := mv.TableView.PrimarySelection()
	value := mv.response.Entries[row][col].Format("")
	// TODO Linux implementation
	cmd := exec.Command("/usr/bin/pbcopy")
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
	row, column := mv.TableView.PrimarySelection()
	primary := mv.Table.Primary()
	key := mv.response.Entries[row][primary].Format("")
	err := mv.Table.Update(key, mv.Table.Columns[column], contents)
	if err != nil {
		return err
	}
	return mv.updateTableViewContents()
}

func (mv *MainView) pasteValue() error {
	// TODO Linux implementation
	cmd := exec.Command("/usr/bin/pbpaste")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return mv.updateEntryValue(strings.TrimSpace(string(out)))
}

func (mv *MainView) editWorkspace() error {
	row, col := mv.TableView.PrimarySelection()
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
