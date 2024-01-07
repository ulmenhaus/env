package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/jql/dbms"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/types"
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

type jqlConfig struct {
	path  string
	table string
	pk    string

	mode string
	addr string
}

func runCLI() error {
	cfg := jqlConfig{}

	var cmd = &cobra.Command{
		Use:   "jql <path> <table> [pk]",
		Short: "The JSON backed smart spreadsheets",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cfg.path = args[0]
			cfg.table = args[1]
			if len(args) > 2 {
				cfg.pk = args[2]
			}
		},
	}

	cmd.Flags().StringVarP(&cfg.mode, "mode", "m", "standalone", "Mode of operation")
	cmd.Flags().StringVarP(&cfg.addr, "addr", "a", "", "Address (for remote connections)")

	if err := cmd.Execute(); err != nil {
		return err
	}
	return runJQL(cfg)
}

func runJQL(cfg jqlConfig) error {
	mapper, db, err := runDatabse(cfg.path)
	if err != nil {
		return err
	}
	// TODO path should be hidden behind the OSM and never used directly by any of these components
	switch cfg.mode {
	case "standalone":
		return runStandalone(cfg.path, cfg.table, mapper, db)
	case "daemon":
		return runDaemon(cfg.path, cfg.table, mapper, db, cfg.addr)
	case "client":
		return runClient(cfg.path, cfg.table, mapper, db)
	default:
		return fmt.Errorf("Unknown mode: %v", cfg.mode)
	}
}

func runDaemon(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database, addr string) error {
	server, err := dbms.NewDatabaseServer(mapper, db, dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database server: %v", err)
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	jqlpb.RegisterJQLServer(s, server)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

func runClient(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database) error {
	return fmt.Errorf("Client mode not yet implemented")
}

func runStandalone(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database) error {
	mv, err := ui.NewMainView(dbPath, tableName, mapper, db)
	if err != nil {
		return err
	}
	if len(os.Args) > 3 {
		pk := os.Args[3]
		err = mv.GoToPrimaryKey(pk)
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

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func runDatabse(path string) (*osm.ObjectStoreMapper, *types.Database, error) {
	mapper, err := osm.NewObjectStoreMapper(path)
	if err != nil {
		return nil, nil, err
	}
	db, err := mapper.Load()
	if err != nil {
		return nil, nil, err
	}
	return mapper, db, err
}
