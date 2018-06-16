package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/ui"
)

func main() {
	// TODO use a cli library
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
	t := &ui.Table{
		Values: [][]string{},
		Widths: []int{},
	}

	header := []string{}
	for _, column := range table.Columns {
		if strings.HasPrefix(column, "_") {
			// TODO note these to skip the values as well
			continue
		}
		t.Widths = append(t.Widths, 30)
		header = append(header, column)
	}
	t.Values = append(t.Values, header)

	for _, row := range table.Entries {
		formatted := []string{}
		for _, entry := range row {
			// TODO extract actual formatting
			formatted = append(formatted, entry.Format(""))
		}
		t.Values = append(t.Values, formatted)
	}

	defer g.Close()

	g.SetManagerFunc(t.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, t.Down); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, t.Up); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowRight, gocui.ModNone, t.Right); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowLeft, gocui.ModNone, t.Left); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", 'o', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		_, err := exec.Command("open", t.GetSelected()[0][0]).CombinedOutput()
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
