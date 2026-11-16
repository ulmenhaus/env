// Package app wires the feed UI into a gocui.Gui so that it may be run either as the
// standalone feed binary or alongside the other timedb tools in a single process
package app

import (
	"context"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/feed/ui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/lib/go/timedb"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

// Tool is the feed UI
type Tool struct {
	cfg  *cli.JQLConfig
	dbms api.JQL_DBMS
	mv   *ui.MainView
}

// New returns the feed tool with the given channel initially selected
func New(g *gocui.Gui, cfg *cli.JQLConfig, dbms api.JQL_DBMS, selected string) (*Tool, error) {
	mv, err := ui.NewMainView(g, dbms, "feed_ignored.json", []string{"--path", cfg.Path}, selected)
	if err != nil {
		return nil, err
	}
	return &Tool{cfg: cfg, dbms: dbms, mv: mv}, nil
}

// Layout lays out the tool's views
func (t *Tool) Layout(g *gocui.Gui) error {
	return t.mv.Layout(g)
}

// Activate is called when switching to an already initialized feed tool. It reloads
// data and selects the requested channel if it exists. It returns true iff the tool's
// previous view state should be restored.
func (t *Tool) Activate(g *gocui.Gui, req cli.SwitchRequest) (bool, error) {
	selected, err := t.mv.Reload(g, req.PK)
	return !selected, err
}

func (t *Tool) persist() error {
	_, err := t.dbms.Persist(context.Background(), &jqlpb.PersistRequest{})
	if err != nil {
		return err
	}
	return t.mv.SaveIgnores()
}

// Bind sets the tool's keybindings on the gui
func (t *Tool) Bind(g *gocui.Gui) error {
	mv, cfg := t.mv, t.cfg
	g.InputEsc = true

	if err := mv.SetKeyBindings(g); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	cycler := func(tool string) func(g *gocui.Gui, v *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			if err := t.persist(); err != nil {
				return err
			}
			return cfg.CycleTool(tool, "")
		}
	}

	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, cycler("execute")); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, cycler("jql")); err != nil {
		return err
	}

	goToSelectedPK := func(g *gocui.Gui, v *gocui.View) error {
		if mv.Mode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'g', gocui.ModNone)
			return nil
		}
		pk, err := mv.GetSelectedPK(g, v)
		if err != nil {
			return err
		}
		cfg.Table = timedb.TableNouns
		if err := t.persist(); err != nil {
			return err
		}
		return cfg.SwitchTool("jql", pk)
	}
	if err := g.SetKeybinding("", 'g', gocui.ModNone, goToSelectedPK); err != nil {
		return err
	}

	goToSkillsetPractices := func(g *gocui.Gui, v *gocui.View) error {
		if mv.Mode != ui.MainViewModeListBar {
			// Not navigating the list (e.g. typing a new entry's
			// description) so let the view handle the keystroke
			mv.Edit(v, gocui.Key(0), 'S', gocui.ModNone)
			return nil
		}
		pk, err := mv.GetSelectedPK(g, v)
		if err != nil {
			return err
		}
		cfg.Table = timedb.TablePractices
		if err := t.persist(); err != nil {
			return err
		}
		return cfg.SwitchTool("jql", "", cli.Filter{
			Key:   timedb.FieldDirect,
			Value: pk,
		})
	}
	return g.SetKeybinding("", 'S', gocui.ModNone, goToSkillsetPractices)
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
