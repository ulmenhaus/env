package main

import (
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/runner/ui"
)

func main() {
	// TODO use a cli library
	dbPath := os.Args[1]
	jqlBinDir := os.Args[2]
	var defaultResourceFilter string
	if len(os.Args) > 3 {
		defaultResourceFilter = os.Args[3]
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}

	// TODO decent amount of common set-up logic here to maybe break into a common subroutine
	defer g.Close()
	mv, err := ui.NewMainView(dbPath, g, jqlBinDir, defaultResourceFilter)
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
