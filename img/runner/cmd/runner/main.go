package main

import (
	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/img/runner/ui"
)

func main() {
	err := runExecute()
	if err != nil {
		panic(err)
	}
}

func runExecute() error {
	cfg := &cli.JQLConfig{}

	var cmd = &cobra.Command{
		Use:   "runner",
		Short: "Presents a convenient view for opening the resources associated with a timedb entry",
	}
	cfg.Register(cmd.Flags())

	var jqlBinDir, initResource, initQuery, initType string

	cmd.Flags().StringVarP(&jqlBinDir, "jql-bin-dir", "d", "", "Directory containing jql binaries")
	cmd.Flags().StringVarP(&initResource, "init-resource", "r", "", "The resource to start at")
	cmd.Flags().StringVarP(&initQuery, "init-query", "q", "", "The initial search query")
	cmd.Flags().StringVarP(&initType, "init-type", "e", "", "The initially selected type of resource")

	if err := cmd.Execute(); err != nil {
		return err
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}

	dbms, err := cfg.InitDBMS()
	if err != nil {
		return err
	}
	// TODO decent amount of common set-up logic here to maybe break into a common subroutine
	defer g.Close()
	mv, err := ui.NewMainView(g, dbms, jqlBinDir, initResource, initQuery, initType)
	if err != nil {
		return err
	}
	g.InputEsc = true

	g.SetManagerFunc(mv.Layout)

	err = mv.SetKeyBindings(g)
	if err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
