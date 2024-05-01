package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

const (
	persistencePath = ".execute.reentrance.json"
)

type ReEntranceContext struct {
	SelectedItem       string
	AttemptingToInject bool
}

func persistReEntranceContext(c *ReEntranceContext) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(persistencePath, data, 0644)
}

func loadAndDeleteReEntranceContext() (ReEntranceContext, error) {
	c := ReEntranceContext{}
	data, err := ioutil.ReadFile(persistencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		} else {
			return c, err
		}
	}
	err = json.Unmarshal(data, &c)
	if err != nil {
		return c, err
	}
	err = os.Remove(persistencePath)
	if err != nil {
		return c, err
	}
	return c, nil
}
