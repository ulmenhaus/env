package main

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/execute/ui"
	"github.com/ulmenhaus/env/img/lib/timedb"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/protobuf/proto"
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
	c, err := loadAndDeleteReEntranceContext()
	if err != nil {
		return err
	}
	mv, err := ui.NewMainView(g, dbms, c.SelectedItem, c.AttemptingToInject)
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
			return cfg.SwitchTool(tool, "")
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
								Values: []string{"Awaiting", "Ready"},
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
			return cfg.SwitchTool("jql", "", cli.Filter{
				Key:   "Parent",
				Value: info.TaskPK,
			})
		} else if info.IsWarmup {
			cfg.Table = timedb.TableTools
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
			return cfg.SwitchTool("jql", "", cli.Filter{
				Key:   timedb.FieldSkillset,
				Value: fmt.Sprintf("@{nouns %s}", info.Skillset),
			})
		}
		return nil
	}
	err = g.SetKeybinding(timedb.TasksView, 'S', gocui.ModNone, substituteOrGoSelect)
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
