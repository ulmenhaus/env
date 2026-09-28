// jql-codedb-navigate is a TUI for interactively exploring the graph
// produced by jql-codedb-extract: a list of components in the middle, what
// the selected component references to the right, and what references it
// to the left.
package main

import (
	"fmt"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/codenav/ui"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <database.json>\n", os.Args[0])
		os.Exit(1)
	}
	dbPath := os.Args[1]

	mv, err := ui.NewMainView(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load %s: %v\n", dbPath, err)
		os.Exit(1)
	}

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	defer g.Close()

	g.InputEsc = true
	g.Highlight = true
	g.SelFgColor = ui.FocusColor
	g.SetManagerFunc(mv.Layout)

	if err := mv.SetKeyBindings(g); err != nil {
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
