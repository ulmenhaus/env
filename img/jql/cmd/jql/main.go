package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/img/jql/ui"
)

func main() {
	// TODO use a cli library
	// XXX refactor
	dbPath := os.Args[1]
	tableName := os.Args[2]
	var store storage.Store
	if !strings.HasSuffix(dbPath, ".json") {
		panic("unknown file type")
	} else {
		store = &storage.JSONStore{}
	}
	mapper, err := osm.NewObjectStoreMapper(store)
	if err != nil {
		panic(err)
	}
	f, err := os.Open(dbPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		panic(err)
	}
	table, ok := db[tableName]
	if !ok {
		panic("unknown table")
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	tv := &ui.TableView{
		Values: [][]string{},
		Widths: []int{},
	}

	mv := &ui.MainView{
		TableView: tv,
	}

	filters := []types.Filter{}
	orderBy := ""
	dec := false

	columns := []string{}
	for _, column := range table.Columns {
		if strings.HasPrefix(column, "_") {
			// TODO note these to skip the values as well
			continue
		}
		tv.Widths = append(tv.Widths, 20)
		columns = append(columns, column)
	}

	entries := [][]types.Entry{}
	refreshTable := func() error {
		tv.Values = [][]string{}
		// NOTE putting this here to support swapping columns later
		header := []string{}
		for _, col := range columns {
			if orderBy == col {
				if dec {
					col += " ^"
				} else {
					col += " v"
				}
			}
			header = append(header, col)
		}
		tv.Header = header

		entries, err = table.Query(filters, orderBy, dec)
		if err != nil {
			return err
		}
		for _, row := range entries {
			// TODO ignore hidden columns
			formatted := []string{}
			for _, entry := range row {
				// TODO extract actual formatting
				formatted = append(formatted, entry.Format(""))
			}
			tv.Values = append(tv.Values, formatted)
		}
		return nil
	}
	refreshTable()
	primary := table.Primary()

	defer g.Close()

	g.SetManagerFunc(mv.Layout)

	if err := g.SetKeybinding("", ':', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if mv.Mode != ui.MainViewModePrompt {
			mv.Mode = ui.MainViewModePrompt
		}
		return nil
	}); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'o', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		_, col := tv.GetSelected()
		orderBy = columns[col]
		dec = false
		return refreshTable()
	}); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'O', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		_, col := tv.GetSelected()
		orderBy = columns[col]
		dec = true
		return refreshTable()
	}); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'i', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		row, col := tv.GetSelected()
		key := entries[row][primary].Format("")
		// TODO should use an Update so table can modify any necessary internals
		new, err := table.Entries[key][col].Add(1)
		if err != nil {
			// TODO show error in prompt
			return err
		}
		table.Entries[key][col] = new
		return refreshTable()
	}); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'I', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		row, col := tv.GetSelected()
		key := entries[row][primary].Format("")
		// TODO should use an Update so table can modify any necessary internals
		new, err := table.Entries[key][col].Add(-1)
		if err != nil {
			// TODO show error in prompt
			return err
		}
		table.Entries[key][col] = new
		return refreshTable()
	}); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'b', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		row, column := tv.GetSelected()
		_, err := exec.Command("open", tv.Values[row][column]).CombinedOutput()
		// TODO error needs to be shown on prompt
		return err
	}); err != nil {
		panic(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		panic(err)
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
