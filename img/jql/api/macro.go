package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

type MacroResponseFilter struct {
	Field     string `json:"field"`
	Formatted string `json:"formatted"`
}

type MacroCurrentView struct {
	Table            string              `json:"table"`
	PKs              []string            `json:"pks"`
	PrimarySelection string              `json:"primary_selection"`
	PrimaryColumn    string              `json:"primary_column"`
	Filter           MacroResponseFilter `json:"filter"`
	OrderBy          string              `json:"order_by"`
	OrderDec         bool                `json:"order_dec"`
}

type MacroInterface struct {
	Snapshot    string           `json:"snapshot"`
	Address     string           `json:"address"`
	CurrentView MacroCurrentView `json:"current_view"`
}

func RunMacro(ctx context.Context, dbms JQL_DBMS, command string, currentView MacroCurrentView, v2 bool) (*MacroInterface, error) {
	var stdout, stderr bytes.Buffer
	input := MacroInterface{
		CurrentView: currentView,
	}
	if v2 {
		switch typed := dbms.(type) {
		case *LocalDBMS:
			snapResp, err := dbms.GetSnapshot(ctx, &jqlpb.GetSnapshotRequest{})
			if err != nil {
				return nil, fmt.Errorf("Could not create snapshot: %s", err)
			}
			input.Snapshot = string(snapResp.Snapshot)
		case *RemoteDBMS:
			input.Address = typed.Address
		default:
			return nil, fmt.Errorf("Unknown dbms type for v2 macro: %T", dbms)
		}
	} else {
		snapResp, err := dbms.GetSnapshot(ctx, &jqlpb.GetSnapshotRequest{})
		if err != nil {
			return nil, fmt.Errorf("Could not create snapshot: %s", err)
		}
		input.Snapshot = string(snapResp.Snapshot)
	}
	inputEncoded, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal input: %s", err)
	}
	parts := strings.Split(command, " ")
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = bytes.NewBuffer(inputEncoded)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		// TODO log stderr
		writeErr := ioutil.WriteFile("/tmp/error.log", stderr.Bytes(), os.ModePerm)
		if writeErr != nil {
			return nil, fmt.Errorf("Could not run macro or store stderr: %s", err)
		}
		return nil, fmt.Errorf("Could not run macro: %s -- error at /tmp/error.log", err)
	}
	var newDB []byte
	var output MacroInterface

	// TODO change to three valued "Output" field: file, stdout, none
	err = json.Unmarshal(stdout.Bytes(), &output)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal macro output: %s", err)
	}
	newDB = []byte(output.Snapshot)

	if v2 {
		switch dbms.(type) {
		case *LocalDBMS:
			_, err = dbms.LoadSnapshot(ctx, &jqlpb.LoadSnapshotRequest{
				Snapshot: newDB,
			})
			if err != nil {
				return nil, fmt.Errorf("Could not load database from macro: %s", err)
			}
		default:
			// TODO pass remove info
		}
	} else {
		_, err = dbms.LoadSnapshot(ctx, &jqlpb.LoadSnapshotRequest{
			Snapshot: newDB,
		})
		if err != nil {
			return nil, fmt.Errorf("Could not load database from macro: %s", err)
		}
	}
	return &output, nil
}
