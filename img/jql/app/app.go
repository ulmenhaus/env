// Package app wires the jql UI into a gocui.Gui so that it may be run either as the
// standalone jql binary or alongside the other timedb tools in a single process
package app

import (
	"context"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/img/jql/ui"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

// Tool is the jql UI
type Tool struct {
	cfg  *cli.JQLConfig
	dbms api.JQL_DBMS
	mv   *ui.MainView
}

// New returns the jql tool initialized from the config, starting on the provided pk
// with the provided filters
func New(cfg *cli.JQLConfig, dbms api.JQL_DBMS, pk string, filters []cli.Filter) (*Tool, error) {
	mv, err := newMainView(cfg, dbms, pk, filters)
	if err != nil {
		return nil, err
	}
	return &Tool{cfg: cfg, dbms: dbms, mv: mv}, nil
}

func newMainView(cfg *cli.JQLConfig, dbms api.JQL_DBMS, pk string, filters []cli.Filter) (*ui.MainView, error) {
	var initialQuery *jqlpb.ListRowsRequest
	if cfg.Query != "" {
		q, err := cfg.GetQuery()
		if err != nil {
			return nil, err
		}
		initialQuery = q
		cfg.Table = q.Table
		// Reset the query so that it won't be carried over in
		// subsequent UI changes
		cfg.Query = ""
	}
	mv, err := ui.NewMainView(dbms, cfg.Table)
	if err != nil {
		return nil, err
	}
	if pk != "" {
		err = mv.GoToPrimaryKey(pk)
		if err != nil {
			return nil, err
		}
	}
	if initialQuery != nil {
		err = mv.LoadQuery(initialQuery)
		if err != nil {
			return nil, err
		}
	} else if len(filters) > 0 {
		err = mv.AddFilters(filters)
		if err != nil {
			return nil, err
		}
	}
	if cfg.OrderBy != "" {
		err = mv.SetOrderBy(cfg.OrderBy, cfg.OrderDec)
		if err != nil {
			return nil, err
		}
		// Reset so that it won't be carried over in subsequent UI changes
		cfg.OrderBy = ""
		cfg.OrderDec = false
	}
	if cfg.GroupByField != "" {
		err = mv.SetGroupBy(cfg.GroupByField, cfg.GroupBySelected)
		if err != nil {
			return nil, err
		}
		// Reset so that it won't be carried over in subsequent UI changes
		cfg.GroupByField = ""
		cfg.GroupBySelected = ""
	}
	if cfg.VerticalGrouping {
		// Applied after the query and groupings since setting either of
		// those resets the vertical grouping mode
		mv.SetVerticalGrouping(true)
		// Reset so that it won't be carried over in subsequent UI changes
		cfg.VerticalGrouping = false
	}
	if cfg.SelectPK != "" {
		mv.SetSelectedPK(cfg.SelectPK)
		// Reset so that it won't be carried over in subsequent UI changes
		cfg.SelectPK = ""
	}
	return mv, nil
}

// Layout lays out the tool's views
func (t *Tool) Layout(g *gocui.Gui) error {
	return t.mv.Layout(g)
}

// Activate is called when switching to an already initialized jql tool. Cycling to jql
// keeps its current state while navigating to jql rebuilds it from the config. It
// returns true iff the tool's previous view state should be restored.
func (t *Tool) Activate(g *gocui.Gui, req cli.SwitchRequest) (bool, error) {
	if req.Cycle {
		return true, t.mv.Refresh()
	}
	mv, err := newMainView(t.cfg, t.dbms, req.PK, req.Filters)
	if err != nil {
		return false, err
	}
	t.mv = mv
	return false, nil
}

// Bind sets the tool's keybindings on the gui
func (t *Tool) Bind(g *gocui.Gui) error {
	g.InputEsc = true
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	cycler := func(envvar, defaultVal string) func(g *gocui.Gui, v *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			_, err := t.dbms.Persist(context.Background(), &jqlpb.PersistRequest{})
			if err != nil {
				return err
			}
			tool := os.Getenv(envvar)
			if tool == "" {
				tool = defaultVal
			}
			return t.cfg.CycleTool(tool, t.mv.GetPrimarySelection())
		}
	}

	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, cycler("JQL_FORWARD_TOOL", "feed")); err != nil {
		return err
	}
	return g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, cycler("JQL_REVERSE_TOOL", "execute"))
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
