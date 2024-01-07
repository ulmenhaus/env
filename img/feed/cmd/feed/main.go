package main

import (
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/feed/ui"
	"github.com/ulmenhaus/env/img/jql/osm"
)

// TODO now that jql is providing a library for other components it would
// be good to factor out and write interfaces for all core libraries in
// jql. Consider even daemonizing jql and having various UIs on top of it.
func main() {
	// TODO use a cli library
	dbPath := os.Args[1]
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	defer g.Close()

	mapper, err := osm.NewObjectStoreMapper(dbPath)
	if err != nil {
		panic(err)
	}
	err = mapper.Load()
	if err != nil {
		panic(err)
	}
	mv, err := ui.NewMainView(g, mapper, dbPath+".ignored", []string{dbPath})
	if err != nil {
		panic(err)
	}
	g.InputEsc = true

	g.SetManagerFunc(mv.Layout)

	err = mv.SetKeyBindings(g)
	if err != nil {
		panic(err)
	}

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
