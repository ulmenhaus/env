package main

import (
	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/feed/app"
	"github.com/ulmenhaus/env/img/jql/cli"
)

func main() {
	err := runFeed()
	if err != nil {
		panic(err)
	}
}

func runFeed() error {
	cfg := &cli.JQLConfig{}

	var cmd = &cobra.Command{
		Use:   "execute",
		Short: "Presents a convenient view of your tasks",
	}
	cfg.Register(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		return err
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}
	defer g.Close()
	dbms, err := cfg.InitDBMS()
	if err != nil {
		return err
	}
	tool, err := app.New(g, cfg, dbms, cfg.PK)
	if err != nil {
		return err
	}
	g.SetManagerFunc(tool.Layout)
	if err := tool.Bind(g); err != nil {
		return err
	}
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}
