package main

import (
	"context"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/feed/ui"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
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
	mv, err := ui.NewMainView(g, dbms, "feed_ignored.json", []string{"--path", cfg.Path}, cfg.PK)
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

	cycler := func(tool string) func(g *gocui.Gui, v *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			_, err := dbms.Persist(context.Background(), &jqlpb.PersistRequest{})
			if err != nil {
				return err
			}
			err = mv.SaveIgnores()
			if err != nil {
				return err
			}
			return cfg.SwitchTool(tool, "")
		}
	}

	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, cycler("execute")); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, cycler("jql")); err != nil {
		return err
	}

	goToSelectedPK := func(g *gocui.Gui, v *gocui.View) error {
		pk, err := mv.GetSelectedPK(g, v)
		if err != nil {
			return err
		}
		cfg.Table = ui.TableNouns
		_, err = dbms.Persist(context.Background(), &jqlpb.PersistRequest{})
		if err != nil {
			return err
		}
		err = mv.SaveIgnores()
		if err != nil {
			return err
		}
		return cfg.SwitchTool("jql", pk)
	}
	err = g.SetKeybinding("", 'g', gocui.ModNone, goToSelectedPK)
	if err != nil {
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
