package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
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
)

// A MainView is the overall view of the table including headers,
// prompts, &c. It will also be responsible for managing differnt
// interaction modes if jql supports those.
type MainView struct {
	Table   *types.Table
	Params  types.QueryParams
	columns []string
	// TODO map[string]types.Entry and []types.Entry could both
	// be higher-level types (e.g. VerboseRow and Row)
	entries [][]types.Entry

	TableView     *TableView
	PromptHandler *PromptHandler
	Mode          MainViewMode

	switching bool // on when transitioning modes has not yet been acknowleged by Layout
	alert     string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(t *types.Table) (*MainView, error) {
	columns := []string{}
	widths := []int{}
	for _, column := range t.Columns {
		if strings.HasPrefix(column, "_") {
			// TODO note these to skip the values as well
			continue
		}
		widths = append(widths, 20)
		columns = append(columns, column)
	}

	mv := &MainView{
		Table: t,
		TableView: &TableView{
			Values: [][]string{},
			Widths: widths,
		},
		columns: columns,
	}
	return mv, mv.updateTableViewContents()
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
		prompt.Editor = mv.PromptHandler
	}
	if switching {
		prompt.Clear()
	}
	switch mv.Mode {
	case MainViewModeTable:
		if _, err := g.SetCurrentView("table"); err != nil {
			return err
		}
		g.InputEsc = false
		g.Cursor = false
	case MainViewModeAlert:
		if _, err := g.SetCurrentView("table"); err != nil {
			return err
		}
		g.InputEsc = true
		g.Cursor = false
		fmt.Fprintf(prompt, mv.alert)
	case MainViewModePrompt:
		if _, err := g.SetCurrentView("prompt"); err != nil {
			return err
		}
		g.InputEsc = true
		g.Cursor = true
	}
	return nil
}

// switchMode sets the main view's mode to the new mode and sets
// the switching flag so that Layout is aware of the transition
func (mv *MainView) switchMode(new MainViewMode) {
	mv.switching = true
	mv.Mode = new
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
	}

	primary := mv.Table.Primary()

	switch ch {
	case 'b':
		row, column := mv.TableView.GetSelected()
		_, err = exec.Command("open", mv.TableView.Values[row][column]).CombinedOutput()
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
		row, col := mv.TableView.GetSelected()
		key := mv.entries[row][primary].Format("")
		// TODO should use an Update so table can modify any necessary internals
		new, err := mv.Table.Entries[key][col].Add(1)
		if err != nil {
			return
		}
		mv.Table.Entries[key][col] = new
		err = mv.updateTableViewContents()
	case 'I':
		row, col := mv.TableView.GetSelected()
		key := mv.entries[row][primary].Format("")
		// TODO should use an Update so table can modify any necessary internals
		new, err := mv.Table.Entries[key][col].Add(-1)
		if err != nil {
			return
		}
		mv.Table.Entries[key][col] = new
		err = mv.updateTableViewContents()
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
