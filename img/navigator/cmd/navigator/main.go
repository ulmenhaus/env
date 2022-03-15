package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/navigator/ui"
)

func main() {
	// HACK ignoring error as tmux is expected to exit 127
	tmuxCmd := exec.Command(ui.TMUX_PATH, "run", ":#W")
	tmuxOut, err := tmuxCmd.Output()
	parts := strings.Split(string(tmuxOut), "'")
	projectName := parts[1][1:]
	// TODO use a cli library
	jqlBinDir := os.Args[1]
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}

	// TODO decent amount of common set-up logic here to maybe break into a common subroutine
	defer g.Close()
	mv, err := ui.NewMainView(g, projectName, jqlBinDir)
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
