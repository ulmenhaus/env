// jql-timedb-client runs the timedb tools (jql, feed, and execute) in a single process
// sharing one DBMS connection. Switching between tools (e.g. with Tab and Esc) swaps the
// active tool in-memory rather than exec'ing a new binary, so each tool keeps its state.
package main

import (
	"fmt"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	execute "github.com/ulmenhaus/env/img/execute/app"
	feed "github.com/ulmenhaus/env/img/feed/app"
	"github.com/ulmenhaus/env/img/jql/api"
	jql "github.com/ulmenhaus/env/img/jql/app"
	"github.com/ulmenhaus/env/img/jql/cli"
)

func main() {
	err := run()
	if err != nil {
		panic(err)
	}
}

func run() error {
	cfg := &cli.JQLConfig{}
	var start string

	var cmd = &cobra.Command{
		Use:   "jql-timedb-client",
		Short: "Runs jql, feed, and execute in a single process",
	}
	cfg.Register(cmd.Flags())
	cmd.Flags().StringVarP(&start, "tool", "", "jql", "The tool to start on (jql, feed, or execute)")

	if err := cmd.Execute(); err != nil {
		return err
	}
	if cfg.Mode == cli.ModeDaemon {
		return fmt.Errorf("jql-timedb-client does not support daemon mode")
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
	c := newClient(g, cfg, dbms)
	if _, ok := c.slots[start]; !ok {
		return fmt.Errorf("unknown tool: %s", start)
	}
	cfg.Switcher = c.requestSwitch
	if err := c.activate(c.slots[start], cli.SwitchRequest{
		Tool:    start,
		PK:      cfg.PK,
		Filters: cfg.GetFilters(),
	}); err != nil {
		return err
	}
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

// A tool is one of the timedb UIs that can be swapped into the gui
type tool interface {
	Layout(g *gocui.Gui) error
	Bind(g *gocui.Gui) error
	// Activate is called when switching back to a tool that has already been
	// initialized. It returns true iff the tool's saved view state should be restored.
	Activate(g *gocui.Gui, req cli.SwitchRequest) (bool, error)
}

type viewState struct {
	cx, cy, ox, oy int
}

// A slot holds a tool, which is lazily initialized the first time it is switched to,
// along with the view state it had when last switched away from
type slot struct {
	init func(req cli.SwitchRequest) (tool, error)
	tool tool

	views       map[string]viewState
	currentView string
}

func (s *slot) save(g *gocui.Gui) {
	s.views = map[string]viewState{}
	for _, v := range g.Views() {
		cx, cy := v.Cursor()
		ox, oy := v.Origin()
		s.views[v.Name()] = viewState{cx: cx, cy: cy, ox: ox, oy: oy}
	}
	s.currentView = ""
	if v := g.CurrentView(); v != nil {
		s.currentView = v.Name()
	}
}

// restore re-applies the saved view state to any views that exist after a layout
func (s *slot) restore(g *gocui.Gui) {
	for name, state := range s.views {
		v, err := g.View(name)
		if err != nil {
			continue
		}
		// Errors are ignored since the view may have changed size or contents
		// in which case its default position is the best we can do
		_ = v.SetOrigin(state.ox, state.oy)
		_ = v.SetCursor(state.cx, state.cy)
	}
	if s.currentView != "" {
		if _, err := g.View(s.currentView); err == nil {
			_, _ = g.SetCurrentView(s.currentView)
		}
	}
}

type client struct {
	g      *gocui.Gui
	slots  map[string]*slot
	active *slot
	// restorePending is true when the active tool's saved view state should be
	// re-applied once its views have been created by a layout
	restorePending bool
}

func newClient(g *gocui.Gui, cfg *cli.JQLConfig, dbms api.JQL_DBMS) *client {
	return &client{
		g: g,
		slots: map[string]*slot{
			"jql": {init: func(req cli.SwitchRequest) (tool, error) {
				return jql.New(cfg, dbms, req.PK, req.Filters)
			}},
			"feed": {init: func(req cli.SwitchRequest) (tool, error) {
				return feed.New(g, cfg, dbms, req.PK)
			}},
			"execute": {init: func(req cli.SwitchRequest) (tool, error) {
				return execute.New(g, cfg, dbms)
			}},
		},
	}
}

// requestSwitch is the config's Switcher. The switch is deferred until the current
// event has been handled because gocui continues to run the remaining keybindings for
// an event after a handler returns, and those must not see the new tool's bindings.
func (c *client) requestSwitch(req cli.SwitchRequest) error {
	target, ok := c.slots[req.Tool]
	if !ok {
		return cli.ErrSwitchUnhandled
	}
	c.g.Update(func(g *gocui.Gui) error {
		return c.activate(target, req)
	})
	return nil
}

func (c *client) activate(target *slot, req cli.SwitchRequest) error {
	if c.active != nil {
		c.active.save(c.g)
	}
	// Setting the manager removes all of the previous tool's views and keybindings
	c.g.SetManagerFunc(c.layout)
	c.g.Cursor = false
	restore := false
	if target.tool == nil {
		t, err := target.init(req)
		if err != nil {
			return err
		}
		target.tool = t
	} else {
		var err error
		restore, err = target.tool.Activate(c.g, req)
		if err != nil {
			return err
		}
	}
	c.active = target
	c.restorePending = restore
	return target.tool.Bind(c.g)
}

func (c *client) layout(g *gocui.Gui) error {
	if c.active == nil {
		return nil
	}
	if err := c.active.tool.Layout(g); err != nil {
		return err
	}
	if !c.restorePending {
		return nil
	}
	c.restorePending = false
	c.active.restore(g)
	// Lay out again so that any contents that depend on cursor positions are redrawn
	return c.active.tool.Layout(g)
}
