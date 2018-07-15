package main

import (
	"os"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
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
	g.InputEsc = true
	mv, err := ui.NewMainView(table)
	if err != nil {
		panic(err)
	}

	defer g.Close()

	g.SetManagerFunc(mv.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		panic(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		panic(err)
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
