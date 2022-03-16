package main

// Provides a basic UI for prompting the user for input and writing to a file

import (
	"io/ioutil"
	"os"

	"github.com/jroimartin/gocui"
)

type MainView struct {
	prompt string
	output string

	input string
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, _ := g.Size()

	prompt, err := g.SetView("prompt", 1, 1, maxX-1, 3)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	prompt.Clear()
	_, err = prompt.Write([]byte(mv.prompt))
	if err != nil {
		return err
	}
	input, err := g.SetView("input", 1, 3, maxX-1, 5)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	input.Editable = true
	input.Editor = mv
	g.SetCurrentView("input")
	input.Clear()
	_, err = input.Write([]byte(mv.input))
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.input) != 0 {
			mv.input = mv.input[:len(mv.input)-1]
		}
	} else if key == gocui.KeySpace {
		mv.input += " "
	} else if key == gocui.KeyEnter {
		mv.saveAndExit()
	} else {
		mv.input += string(ch)
	}
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	return nil
}

func (mv *MainView) saveAndExit() {
	err := ioutil.WriteFile(mv.output, []byte(mv.input), 0644)
	if err != nil {
		panic(err)
	}
	os.Exit(0)
}

func main() {
	// TODO use a cli library
	prompt := os.Args[1]
	output := os.Args[2]
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}

	// TODO decent amount of common set-up logic here to maybe break into a common subroutine
	defer g.Close()
	mv := &MainView{
		prompt: prompt,
		output: output,
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
