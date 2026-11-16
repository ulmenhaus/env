// Package app wires the execute UI into a gocui.Gui so that it may be run either as the
// standalone execute binary or alongside the other timedb tools in a single process
package app

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/execute/ui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/lib/go/timedb"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/protobuf/proto"
)

// Tool is the execute UI
type Tool struct {
	cfg  *cli.JQLConfig
	dbms api.JQL_DBMS
	mv   *ui.MainView
}

// New returns the execute tool, resuming from any persisted re-entrance context
func New(g *gocui.Gui, cfg *cli.JQLConfig, dbms api.JQL_DBMS) (*Tool, error) {
	c, err := loadAndDeleteReEntranceContext()
	if err != nil {
		return nil, err
	}
	mv, err := ui.NewMainView(g, dbms, c.SelectedItem, c.AttemptingToInject)
	if err != nil {
		return nil, err
	}
	return &Tool{cfg: cfg, dbms: dbms, mv: mv}, nil
}

// Layout lays out the tool's views
func (t *Tool) Layout(g *gocui.Gui) error {
	return t.mv.Layout(g)
}

// Activate is called when switching to an already initialized execute tool. It reloads
// data and resumes from any persisted re-entrance context. It returns true iff the tool's
// previous view state should be restored.
func (t *Tool) Activate(g *gocui.Gui, req cli.SwitchRequest) (bool, error) {
	c, err := loadAndDeleteReEntranceContext()
	if err != nil {
		return false, err
	}
	return c.SelectedItem == "", t.mv.Reload(g, c.SelectedItem, c.AttemptingToInject)
}

// Bind sets the tool's keybindings on the gui
func (t *Tool) Bind(g *gocui.Gui) error {
	mv, cfg, dbms := t.mv, t.cfg, t.dbms
	g.InputEsc = true

	err := mv.SetKeyBindings(g)
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
			return cfg.CycleTool(tool, "")
		}
	}

	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, cycler("jql")); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, cycler("feed")); err != nil {
		return err
	}

	goToActiveReminder := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'a', gocui.ModNone)
			return nil
		}
		reminderID, err := mv.GetSelectedReminderArg0(g)
		if err != nil {
			return err
		}
		req := &jqlpb.ListRowsRequest{
			Table:   timedb.TableActiveReminders,
			OrderBy: "A Order",
		}
		data, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		cfg.Table = ""
		cfg.Query = base64.StdEncoding.EncodeToString(data)
		cfg.SelectPK = fmt.Sprintf("%s %s", timedb.TableReminders, reminderID)
		return cfg.SwitchTool("jql", "")
	}
	err = g.SetKeybinding("", 'a', gocui.ModNone, goToActiveReminder)
	if err != nil {
		return err
	}

	goToReminders := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'A', gocui.ModNone)
			return nil
		}
		reminderID, err := mv.GetSelectedReminderArg0(g)
		if err != nil {
			return err
		}
		req := &jqlpb.ListRowsRequest{
			Table:   timedb.TableReminders,
			OrderBy: "A Order",
			Conditions: []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: "Status",
							Match: &jqlpb.Filter_InMatch{InMatch: &jqlpb.InMatch{
								Values: []string{"Ready"},
							}},
						},
					},
				},
			},
		}
		data, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		cfg.Table = ""
		cfg.Query = base64.StdEncoding.EncodeToString(data)
		cfg.SelectPK = fmt.Sprintf("%s %s", timedb.TableReminders, reminderID)
		return cfg.SwitchTool("jql", "")
	}
	err = g.SetKeybinding("", 'A', gocui.ModNone, goToReminders)
	if err != nil {
		return err
	}

	goToSelectedPK := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'g', gocui.ModNone)
			return nil
		}
		pk, err := mv.ResolveSelectedPK(g)
		if err != nil {
			return err
		}
		cfg.Table = timedb.TableTasks
		return cfg.SwitchTool("jql", pk)
	}
	err = g.SetKeybinding("", 'g', gocui.ModNone, goToSelectedPK)
	if err != nil {
		return err
	}

	goToRelatives := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), ',', gocui.ModNone)
			return nil
		}
		taskPK, err := mv.ResolveSelectedPK(g)
		if err != nil {
			return err
		}
		task, err := dbms.GetRow(context.Background(), &jqlpb.GetRowRequest{
			Table: timedb.TableTasks,
			Pk:    taskPK,
		})
		if err != nil {
			return err
		}
		direct := task.Row.Entries[api.IndexOfField(task.Columns, timedb.FieldDirect)].Formatted
		if direct == "" {
			return nil
		}
		req := &jqlpb.ListRowsRequest{
			Table: "vt.relatives",
			Conditions: []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: "-> Item",
							Match: &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{
								Value: fmt.Sprintf("%s %s", timedb.TableNouns, direct),
							}},
						},
					},
				},
			},
			GroupBy: &jqlpb.GroupBy{
				Groupings: []*jqlpb.RequestedGrouping{
					{Field: "Relation"},
				},
			},
			OrderBy: "A Date",
			Dec:     true,
		}
		data, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		cfg.Table = ""
		cfg.Query = base64.StdEncoding.EncodeToString(data)
		return cfg.SwitchTool("jql", "")
	}
	err = g.SetKeybinding("", ',', gocui.ModNone, goToRelatives)
	if err != nil {
		return err
	}

	goToProfiles := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'M', gocui.ModNone)
			return nil
		}
		taskPK, err := mv.ResolveSelectedPK(g)
		if err != nil {
			return err
		}
		req := &jqlpb.ListRowsRequest{
			Table: "vt.profiles",
			Conditions: []*jqlpb.Condition{
				{
					Requires: []*jqlpb.Filter{
						{
							Column: "_target",
							Match: &jqlpb.Filter_InMatch{InMatch: &jqlpb.InMatch{
								Values: []string{fmt.Sprintf("%s %s", timedb.TableTasks, taskPK)},
							}},
						},
					},
				},
			},
			GroupBy: &jqlpb.GroupBy{
				Groupings: []*jqlpb.RequestedGrouping{
					{Field: "Profile"},
				},
			},
		}
		data, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		cfg.Table = ""
		cfg.Query = base64.StdEncoding.EncodeToString(data)
		return cfg.SwitchTool("jql", "")
	}
	err = g.SetKeybinding("", 'M', gocui.ModNone, goToProfiles)
	if err != nil {
		return err
	}

	goToReview := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'R', gocui.ModNone)
			return nil
		}
		req := &jqlpb.ListRowsRequest{
			Table:   "vt.review",
			OrderBy: "Z %",
			Dec:     true,
			GroupBy: &jqlpb.GroupBy{
				Groupings: []*jqlpb.RequestedGrouping{
					{Field: "A Class", Selected: "Stat"},
					{Field: "By", Selected: "Action"},
					{Field: "A Skillset"},
				},
			},
		}
		data, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		cfg.Table = ""
		cfg.Query = base64.StdEncoding.EncodeToString(data)
		cfg.VerticalGrouping = true
		return cfg.SwitchTool("jql", "")
	}
	err = g.SetKeybinding("", 'R', gocui.ModNone, goToReview)
	if err != nil {
		return err
	}

	selectAndGoToTask := func(g *gocui.Gui, v *gocui.View) error {
		if mv.MainViewMode != ui.MainViewModeListBar {
			mv.Edit(v, gocui.Key(0), 'G', gocui.ModNone)
			return nil
		}
		return mv.SelectTask(g, v, func(taskPK string) error {
			cfg.Table = timedb.TableTasks
			return cfg.SwitchTool("jql", taskPK)
		})
	}
	err = g.SetKeybinding("", 'G', gocui.ModNone, selectAndGoToTask)
	if err != nil {
		return err
	}

	substituteOrGoSelect := func(g *gocui.Gui, v *gocui.View) error {
		count, err := mv.InjectTaskWithAllMatching(g, v, false)
		if err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		info, err := mv.GetCurrentDomain(g, v)
		if err != nil {
			return err
		}
		err = persistReEntranceContext(&ReEntranceContext{
			SelectedItem:       info.TaskPK,
			AttemptingToInject: true,
		})
		if err != nil {
			return err
		}
		if info.IsPrepTask {
			cfg.Table = timedb.TableKits
			cfg.OrderBy = timedb.FieldDaysUntil
			cfg.OrderDec = false
			return cfg.SwitchTool("jql", "", cli.Filter{
				Key:   "Parent",
				Value: info.TaskPK,
			})
		} else if info.IsWarmup {
			cfg.Table = timedb.TableTools
			cfg.OrderBy = timedb.FieldDaysUntil
			cfg.OrderDec = false
			return cfg.SwitchTool(
				"jql",
				"",
				cli.Filter{
					Key:   "-> Item",
					Value: info.Direct,
				},
				cli.Filter{
					Key:   "Parent",
					Value: info.TaskPK,
				},
			)
		} else {
			cfg.Table = timedb.TablePractices
			cfg.OrderBy = timedb.FieldDaysUntil
			cfg.OrderDec = false
			cfg.GroupByField = timedb.FieldTowards
			cfg.GroupBySelected = timedb.ValueSomethingNew
			return cfg.SwitchTool("jql", "", cli.Filter{
				Key:   timedb.FieldSkillset,
				Value: fmt.Sprintf("@{nouns %s}", info.Skillset),
			})
		}
		return nil
	}
	return g.SetKeybinding(timedb.TasksView, 'S', gocui.ModNone, substituteOrGoSelect)
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
