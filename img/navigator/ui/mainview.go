package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/img/jql/ui"
)

const (
	// HACK hard-coding path to tmux and emacsclient
	TMUX_PATH         = "/usr/local/bin/tmux"
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

type Resource struct {
	Location    string
	Description string
}

// A MainView is the overall view including a list of resources
type MainView struct {
	Mode   MainViewMode
	TypeIX int

	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	CodeOSM *osm.ObjectStoreMapper
	CodeDB  *types.Database

	projectName     string
	resourceQ       string
	allResources    []Resource
	activeResources []Resource
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, projectName, jqlBinDir string) (*MainView, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	projectsPath := filepath.Join(homedir, ".projects.json")
	mapper, err := osm.NewObjectStoreMapper(&storage.JSONStore{})
	if err != nil {
		return nil, err
	}
	f, err := os.Open(projectsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		return nil, err
	}

	mv := &MainView{
		OSM: mapper,
		DB:  db,

		TypeIX: 1,

		projectName: projectName,
	}

	projWorkdir, err := mv.getProjectWorkdir()
	if err != nil {
		return nil, err
	}
	projectPath := filepath.Join(projWorkdir, ".project.json")
	codeMapper, err := osm.NewObjectStoreMapper(&storage.JSONStore{})
	if err != nil {
		return nil, err
	}
	f, err = os.Open(projectPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if err == nil {
		defer f.Close()
		codeDB, err := mapper.Load(f)
		if err != nil {
			return nil, err
		}
		mv.CodeDB = codeDB
		mv.CodeOSM = codeMapper
	}
	return mv, mv.refreshAllResources()
}

// Edit handles keyboard inputs while searching
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	mv.editQuery(v, key, ch, mod)
	return
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
	mv.refreshActiveResources()
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
	view, err := g.SetView(ResourceView, 0, 3, maxX-1, maxY-3)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	view.Highlight = true
	view.SelBgColor = gocui.ColorWhite
	view.SelFgColor = gocui.ColorBlack
	view.Clear()
	for _, active := range mv.activeResources {
		_, err = view.Write([]byte(active.Description + "\n"))
		if err != nil {
			return err
		}
	}

	subDisplay, err := g.SetView(SubDisplayView, 0, maxY-3, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	subDisplay.Clear()
	subDisplay.Editable = true
	subDisplay.Editor = mv
	if mv.Mode == MainViewModeListResources {
		_, cy := view.Cursor()
		_, oy := view.Origin()
		ix := cy + oy
		if ix < len(mv.activeResources) {
			subDisplay.Write([]byte(mv.activeResources[ix].Location))
		}
	} else if mv.Mode == MainViewModeQueryResources {
		subDisplay.Write([]byte(mv.resourceQ))
	}
	if mv.Mode == MainViewModeListResources {
		g.SetCurrentView(ResourceView)
	} else if mv.Mode == MainViewModeQueryResources {
		g.SetCurrentView(SubDisplayView)
	}
	return nil
}
func (mv *MainView) titleBar(g *gocui.Gui) error {
	maxX, _ := g.Size()
	types := ListResourcesTypes
	for i, t := range types {
		width := maxX / len(types)
		startX := i * width
		view, err := g.SetView(fmt.Sprintf("%s-%s", TypeView, t), startX, 0, startX+width, 2)
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
	err = g.SetKeybinding(ResourceView, 'c', gocui.ModNone, mv.changeDirectory)
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
	mv.resourceQ = ""
	mv.TypeIX = (mv.TypeIX + 1) % len(ListResourcesTypes)
	return mv.refreshAllResources()
}

func (mv *MainView) decrementType(g *gocui.Gui, v *gocui.View) error {
	mv.resourceQ = ""
	mv.TypeIX -= 1
	if mv.TypeIX < 0 {
		mv.TypeIX = len(ListResourcesTypes) - 1
	}
	return mv.refreshAllResources()
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

func (mv *MainView) refreshAllResources() error {
	mv.allResources = []Resource{}
	switch ListResourcesTypes[mv.TypeIX] {
	case ResourceTypeComponents:
		mv.gatherComponents()
	case ResourceTypeBookmarks:
		mv.gatherBookmarks()
	case ResourceTypeJumps:
		mv.gatherJumps()
	}
	return mv.refreshActiveResources()
}

func (mv *MainView) refreshActiveResources() error {
	mv.activeResources = []Resource{}
	for _, resource := range mv.allResources {
		if strings.Contains(strings.ToLower(resource.Description), strings.ToLower(mv.resourceQ)) {
			mv.activeResources = append(mv.activeResources, resource)
		}
	}
	return nil
}

func (mv *MainView) gatherJumps() error {
	jumpsTable := mv.DB.Tables[JumpsTable]
	jumps, err := jumpsTable.Query(
		types.QueryParams{
			Filters: []types.Filter{
				&ui.EqualFilter{
					Field:     FieldProject,
					Col:       jumpsTable.IndexOfField(FieldProject),
					Formatted: mv.projectName,
				},
			},
			OrderBy: FieldOrder,
		},
	)
	if err != nil {
		return err
	}

	for _, jump := range jumps.Entries {
		mv.allResources = append(mv.allResources, Resource{
			// TODO auto-resolve a description based on components
			Description: jump[jumpsTable.Primary()].Format(""),
			Location:    jump[jumpsTable.Primary()].Format(""),
		})
	}
	return nil
}

func (mv *MainView) gatherComponents() error {
	if mv.CodeDB == nil {
		return nil
	}
	componentsTable := mv.CodeDB.Tables[ComponentsTable]
	components, err := componentsTable.Query(
		types.QueryParams{
			OrderBy: FieldDisplayName,
		},
	)
	if err != nil {
		return err
	}

	for _, component := range components.Entries {
		mv.allResources = append(mv.allResources, Resource{
			Description: component[componentsTable.IndexOfField(FieldDisplayName)].Format(""),
			Location:    component[componentsTable.IndexOfField(FieldSrcLocation)].Format(""),
		})
	}
	return nil
}

func (mv *MainView) gatherBookmarks() error {
	bookmarksTable := mv.DB.Tables[BookmarksTable]
	bookmarks, err := bookmarksTable.Query(
		types.QueryParams{
			Filters: []types.Filter{
				&ui.EqualFilter{
					Field:     FieldProject,
					Col:       bookmarksTable.IndexOfField(FieldProject),
					Formatted: mv.projectName,
				},
			},
			OrderBy: FieldOrder,
		},
	)
	if err != nil {
		return err
	}

	for _, bookmark := range bookmarks.Entries {
		mv.allResources = append(mv.allResources, Resource{
			Description: bookmark[bookmarksTable.IndexOfField(FieldDescription)].Format(""),
			Location:    bookmark[bookmarksTable.Primary()].Format(""),
		})
	}
	return nil
}

func (mv *MainView) selectItem(g *gocui.Gui, v *gocui.View) error {
	// TODO should probably re-order these from MRU
	return mv.selectJump(g, v)
}

func (mv *MainView) selectJump(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	jump := mv.activeResources[oy+cy]
	parts := strings.Split(jump.Location, "#")
	path, pos := parts[0], parts[1]
	workdir, err := mv.getProjectWorkdir()
	if err != nil {
		return err
	}
	cmd := exec.Command(EMACS_CLIENT_PATH, "-n", "-s", mv.projectName, path)
	cmd.Dir = workdir
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

func (mv *MainView) changeDirectory(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	res := mv.activeResources[oy+cy]
	parts := strings.Split(res.Location, "#")
	path, _ := parts[0], parts[1]
	workdir, err := mv.getProjectWorkdir()
	if err != nil {
		return err
	}
	dir := filepath.Join(workdir, filepath.Dir(path))

	cmd := exec.Command(TMUX_PATH, "send", "cd", " ", dir, "ENTER")
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
	mv.Mode = MainViewModeQueryResources
	return nil
}

func (mv *MainView) getProjectWorkdir() (string, error) {
	allProjects := mv.DB.Tables[ProjectsTable]
	projects, err := allProjects.Query(
		types.QueryParams{
			Filters: []types.Filter{
				&ui.EqualFilter{
					Field:     FieldProjectName,
					Col:       allProjects.IndexOfField(FieldProjectName),
					Formatted: mv.projectName,
				},
			},
			OrderBy: FieldProjectName,
		},
	)
	if err != nil {
		return "", err
	}
	if projects.Total != 1 {
		return "", fmt.Errorf("Expected one poject with this name, got: %d", projects.Total)
	}
	workdir := projects.Entries[0][allProjects.IndexOfField(FieldWorkdir)].Format("")
	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return strings.Replace(workdir, "~", homedir, 1), nil

}
