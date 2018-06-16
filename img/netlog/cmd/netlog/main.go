package main

import (
	"fmt"
	"log"

	"github.com/jroimartin/gocui"
)

type SelectableList struct {
	items    []string // should be String() interface
	selected int
}

func (sl *SelectableList) Down(g *gocui.Gui, v *gocui.View) error {
	if sl.selected < len(sl.items)-1 {
		sl.selected++
	}
	return nil
}

func (sl *SelectableList) Up(g *gocui.Gui, v *gocui.View) error {
	if sl.selected > 0 {
		sl.selected--
	}
	return nil
}

func (sl *SelectableList) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView("list", 0, 0, maxX-1, maxY-1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	content := ""
	for i, item := range sl.items {
		if i == sl.selected {
			content += ">>> "
		} else {
			content += "    "
		}
		content += item + "\n"
	}
	v.Clear()
	fmt.Fprintf(v, content)
	return nil
}

func main() {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	sl := &SelectableList{
		items: []string{
			"item0",
			"item1",
			"item2",
			"item3",
			"item4",
			"item5",
			"item6",
		},
	}

	defer g.Close()

	g.SetManagerFunc(sl.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, sl.Down); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, sl.Up); err != nil {
		log.Panicln(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
