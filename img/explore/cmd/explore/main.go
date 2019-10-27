package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/explore/models"
	"github.com/ulmenhaus/env/img/explore/ui"
)

func main() {
	// TODO use a cli library
	if len(os.Args) != 2 {
		panic("Usage: explore [graph]")
	}
	path := os.Args[1]
	encoded := &models.EncodedGraph{}
	serialized, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(serialized, encoded)
	if err != nil {
		panic(err)
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	defer g.Close()
	mv, err := ui.NewMainView(models.DecodeGraph(encoded), g)
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
