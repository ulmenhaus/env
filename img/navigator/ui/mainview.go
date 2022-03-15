package ui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jroimartin/gocui"
)

const (
	// HACK hard-coding path to tmux and emacsclient
	TMUX_PATH = "/usr/local/bin/tmux"
	EMACS_CLIENT_PATH = "/usr/local/bin/emacsclient"
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int
type ResourceType string

const (
	MainViewModeListResources MainViewMode = iota
	MainViewModeQueryResources

	ResourceTypeComponents ResourceType = "components"
	ResourceTypeBookmarks  ResourceType = "bookmarks"
	ResourceTypeJumps      ResourceType = "jumps"
)

var (
	ListResourcesTypes = []ResourceType{
		ResourceTypeComponents,
		ResourceTypeBookmarks,
		ResourceTypeJumps,
	}
)

// A MainView is the overall view including a list of resources
type MainView struct {
	Mode   MainViewMode
	TypeIX int

	project         Project
	projectName     string
	resourceQ       string
	activeResources []string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, projectName, jqlBinDir string) (*MainView, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	projectsPath := filepath.Join(homedir, ".projects.json")
	contents, err := ioutil.ReadFile(projectsPath)
	if err != nil {
		return nil, err
	}
	projectsFile := &ProjectsFile{}
	err = json.Unmarshal(contents, projectsFile)
	if err != nil {
		return nil, err
	}
	project, ok := projectsFile.Projects[projectName]
	if !ok {
		return nil, fmt.Errorf("No meta-data for project: %s", projectName)
	}
	mv := &MainView{
		project:     project,
		projectName: projectName,
	}
	return mv, mv.refreshResources()
}

// Edit handles keyboard inputs while searching
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
}

func (mv *MainView) editQuery(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.resourceQ) != 0 {
			mv.resourceQ = mv.resourceQ[:len(mv.resourceQ)-1]
		}
	} else if key == gocui.KeySpace {
		mv.resourceQ += " "
	} else if key == gocui.KeyEnter {
		mv.Mode = MainViewModeListResources
	} else {
		mv.resourceQ += string(ch)
	}
	mv.refreshResources()
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	return mv.listResourcesLayout(g)
}

func (mv *MainView) listResourcesLayout(g *gocui.Gui) error {
	err := mv.titleBar(g)
	if err != nil {
		return err
	}
	maxX, maxY := g.Size()
	view, err := g.SetView(ResourceView, 0, 5, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	view.Highlight = true
	view.SelBgColor = gocui.ColorWhite
	view.SelFgColor = gocui.ColorBlack
	view.Clear()
	for _, active := range mv.activeResources {
		_, err = view.Write([]byte(active + "\n"))
		if err != nil {
			return err
		}
	}
	g.SetCurrentView(ResourceView)

	return nil
}
func (mv *MainView) titleBar(g *gocui.Gui) error {
	maxX, _ := g.Size()
	types := ListResourcesTypes
	for i, t := range types {
		width := maxX / len(types)
		startX := i * width
		view, err := g.SetView(fmt.Sprintf("%s-%s", TypeView, t), startX, 2, startX+width, 4)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		view.Frame = true
		if i == mv.TypeIX {
			view.BgColor = gocui.ColorWhite
			view.FgColor = gocui.ColorBlack
		} else {
			view.BgColor = gocui.ColorBlack
			view.FgColor = gocui.ColorWhite
		}
		view.Clear()
		spaces := (width - len(t)) / 2
		view.Write([]byte(strings.Repeat(" ", spaces) + string(t)))
	}
	return nil
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	err := g.SetKeybinding(ResourceView, 'l', gocui.ModNone, mv.incrementType)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'h', gocui.ModNone, mv.decrementType)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'j', gocui.ModNone, mv.incrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'k', gocui.ModNone, mv.decrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'f', gocui.ModNone, mv.enterSearchMode)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, '/', gocui.ModNone, mv.toggleSearch)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(ResourceView, gocui.KeyEnter, gocui.ModNone, mv.selectItem); err != nil {
		return err
	}
	return nil
}

func (mv *MainView) incrementType(g *gocui.Gui, v *gocui.View) error {
	mv.TypeIX = (mv.TypeIX + 1) % len(ListResourcesTypes)
	return mv.refreshResources()
}

func (mv *MainView) decrementType(g *gocui.Gui, v *gocui.View) error {
	mv.TypeIX -= 1
	if mv.TypeIX < 0 {
		mv.TypeIX = len(ListResourcesTypes) - 1
	}
	return mv.refreshResources()
}

func (mv *MainView) incrementCursor(g *gocui.Gui, v *gocui.View) error {
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	/*
		cap := len(mv.resources)
		view := v.Name()
		if view == TopicView {
			cap = len(mv.topics)
		} else if view == FilterView {
			cap = len(mv.filters)
		}
		if cy+oy == cap-1 {
			return nil
		}
	*/
	if err := v.SetCursor(cx, cy+1); err != nil {
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) decrementCursor(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	cx, cy := v.Cursor()
	if cy+oy == 0 {
		return nil
	}
	if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) refreshResources() error {
	mv.activeResources = []string{}
	for _, jump := range mv.project.Jumps {
		mv.activeResources = append(mv.activeResources, jump)
	}
	return nil
}

func (mv *MainView) gatherResources() error {
	return nil
}

func (mv *MainView) selectItem(g *gocui.Gui, v *gocui.View) error {
	return mv.selectJump(g, v)
}

func (mv *MainView) selectJump(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	jump := mv.activeResources[oy+cy]
	parts := strings.Split(jump, "#")
	path, pos := parts[0], parts[1]
	cmd := exec.Command(EMACS_CLIENT_PATH, "-n", "-s", mv.projectName, path)
	homedir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cmd.Dir = strings.Replace(mv.project.Workdir, "~", homedir, 1)
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command(TMUX_PATH, "send", "Escape", "x", "goto-char", "ENTER", string(pos), "ENTER")
	err = cmd.Run()
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func (mv *MainView) enterSearchMode(g *gocui.Gui, v *gocui.View) error {
	return nil
}

func (mv *MainView) toggleSearch(g *gocui.Gui, v *gocui.View) error {
	return nil
}
