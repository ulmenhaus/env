package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/cli"
	"github.com/ulmenhaus/env/img/jql/ui"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/grpc"
)

func main() {
	err := runCLI()
	if err != nil {
		panic(err)
	}
}

func runCLI() error {
	cfg := &cli.JQLConfig{}

	var cmd = &cobra.Command{
		Use:   "jql",
		Short: "The JSON backed smart spreadsheets",
	}
	cfg.Register(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		return err
	}
	return runJQL(cfg)
}

func runJQL(cfg *cli.JQLConfig) error {
	// TODO path should be hidden behind the OSM and never used directly by any of these components
	switch cfg.Mode {
	case cli.ModeStandalone, cli.ModeClient:
		dbms, _, err := cfg.InitDBMS()
		if err != nil {
			return err
		}
		return runUI(cfg, dbms)
	case cli.ModeDaemon:
		dbms, _, err := cfg.InitDBMS()
		if err != nil {
			return err
		}
		return runDaemon(cfg, dbms)
	default:
		return fmt.Errorf("Unknown mode: %v", cfg.Mode)
	}
}

func runDaemon(cfg *cli.JQLConfig, dbms api.JQL_DBMS) error {
	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer(
		grpc.MaxSendMsgSize(cli.MaxPayloadSize),
		grpc.MaxRecvMsgSize(cli.MaxPayloadSize),
	)
	jqlpb.RegisterJQLServer(s, api.NewDBMSShim(dbms))
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

func runUI(cfg *cli.JQLConfig, dbms api.JQL_DBMS) error {
	mv, err := ui.NewMainView(dbms, cfg.Table)
	if err != nil {
		return err
	}
	if cfg.PK != "" {
		err = mv.GoToPrimaryKey(cfg.PK)
		if err != nil {
			return err
		}
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}
	defer g.Close()
	g.InputEsc = true

	g.SetManagerFunc(mv.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	cycler := func(envvar, defaultVal string) func(g *gocui.Gui, v *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			var tool string
			tool = os.Getenv(envvar)
			if tool == "" {
				tool = defaultVal
			}
			return cfg.SwitchTool(tool)
		}
	}

	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, cycler("JQL_FORWARD_TOOL", "feed")); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, cycler("JQL_REVERSE_TOOL", "execute")); err != nil {
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
