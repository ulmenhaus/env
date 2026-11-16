package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/app"
	"github.com/ulmenhaus/env/img/jql/cli"
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
	switch cfg.Mode {
	case cli.ModeStandalone, cli.ModeClient:
		dbms, err := cfg.InitDBMS()
		if err != nil {
			return err
		}
		return runUI(cfg, dbms)
	case cli.ModeDaemon:
		dbms, err := cfg.InitDBMS()
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
	sizeOpts := []grpc.ServerOption{
		grpc.MaxSendMsgSize(cli.MaxPayloadSize),
		grpc.MaxRecvMsgSize(cli.MaxPayloadSize),
	}
	tcpOpts := append([]grpc.ServerOption{}, sizeOpts...)
	tlsOpt, err := cfg.ServerCredentials()
	if err != nil {
		return err
	}
	if tlsOpt != nil {
		tcpOpts = append(tcpOpts, tlsOpt)
	}
	var backend jqlpb.JQLServer
	backend = api.NewDBMSShim(dbms)
	if cfg.VirtualGateway != "" {
		gateway, err := cfg.InitVirtualDBMS()
		if err != nil {
			return err
		}
		backend = api.NewRouter(backend, api.NewDBMSShim(gateway))
	}
	if cfg.ListenUnix != "" {
		os.Remove(cfg.ListenUnix)
		unixLis, err := net.Listen("unix", cfg.ListenUnix)
		if err != nil {
			return fmt.Errorf("failed to listen on unix socket: %v", err)
		}
		unixServer := grpc.NewServer(sizeOpts...)
		jqlpb.RegisterJQLServer(unixServer, backend)
		log.Printf("server additionally listening at unix://%v", cfg.ListenUnix)
		go unixServer.Serve(unixLis)
	}
	s := grpc.NewServer(tcpOpts...)
	jqlpb.RegisterJQLServer(s, backend)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

func runUI(cfg *cli.JQLConfig, dbms api.JQL_DBMS) error {
	tool, err := app.New(cfg, dbms, cfg.PK, cfg.GetFilters())
	if err != nil {
		return err
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}
	defer g.Close()

	g.SetManagerFunc(tool.Layout)
	if err := tool.Bind(g); err != nil {
		return err
	}
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}
