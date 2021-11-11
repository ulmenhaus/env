package main

import (
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/ui"
)

func main() {
	// TODO use a cli library
	dbPath := os.Args[1]
	tableName := os.Args[2]
	mv, err := ui.NewMainView(dbPath, tableName)
	if err != nil {
		panic(err)
	}
	if len(os.Args) > 3 {
		pk := os.Args[3]
		err = mv.GoToPrimaryKey(pk)
		if err != nil {
			panic(err)
		}
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	defer g.Close()
	g.InputEsc = true

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
