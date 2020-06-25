package format

import (
	"encoding/json"
	"io/ioutil"
	"os/user"
	"path/filepath"
)

type Project struct {
	Bookmarks map[string]interface{} `json:"bookmarks"`
}

type ProjectDatabase struct {
	Projects map[string]Project `json:"projects"`
}

func GetProjectBookmarks(projectName string) (map[string]interface{}, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(usr.HomeDir, ".projects.json")
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	db := ProjectDatabase{}
	err = json.Unmarshal(contents, &db)
	if err != nil {
		return nil, err
	}
	return db.Projects[projectName].Bookmarks, nil
}
