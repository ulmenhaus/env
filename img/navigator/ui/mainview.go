package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
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

type Location struct {
	Path  string
	Point int
}

func (l Location) String() string {
	return fmt.Sprintf("%s#%d", l.Path, l.Point)
}

func NewLocation(str string) Location {
	parts := strings.SplitN(str, "#", 2)
	path, pointS := parts[0], ""
	if len(parts) > 1 {
		pointS = parts[1]
	}
	point, err := strconv.Atoi(pointS)
	if err != nil {
		// NOTE maybe redundant, but explicitly checking error here
		point = 0
	}
	return Location{
		Path:  path,
		Point: point,
	}
}

type Resource struct {
	Location    Location
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

	componentLookup map[string][]Resource // maps each file to a list of components sorted by location
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, projectName, jqlBinDir string) (*MainView, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	projectsPath := filepath.Join(homedir, ".projects.json")
	mapper, err := osm.NewObjectStoreMapper(projectsPath)
	if err != nil {
		return nil, err
	}
	db, err := mapper.Load()
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
	codeMapper, err := osm.NewObjectStoreMapper(projectPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(projectPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if err == nil {
		defer f.Close()
		codeDB, err := mapper.Load()
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
			subDisplay.Write([]byte(mv.activeResources[ix].Location.String()))
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
	err = g.SetKeybinding(ResourceView, 'q', gocui.ModNone, mv.clearSearch)
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
		err := mv.gatherComponents()
		if err != nil {
			return err
		}
	case ResourceTypeBookmarks:
		err := mv.gatherBookmarks()
		if err != nil {
			return err
		}
	case ResourceTypeJumps:
		if mv.componentLookup == nil {
			mv.componentLookup = map[string][]Resource{}
			err := mv.gatherComponents()
			if err != nil {
				return err
			}
			for _, res := range mv.allResources {
				mv.componentLookup[res.Location.Path] = append(mv.componentLookup[res.Location.Path], res)
			}
			for key := range mv.componentLookup {
				sort.Slice(mv.componentLookup[key], func(i, j int) bool {
					return mv.componentLookup[key][i].Location.Point < mv.componentLookup[key][j].Location.Point
				})
			}
			mv.allResources = []Resource{}
		}
		err := mv.gatherJumps()
		if err != nil {
			return err
		}
	}
	return mv.refreshActiveResources()
}

func (mv *MainView) refreshActiveResources() error {
	mv.activeResources = []Resource{}
	// if the source query is all lower case then we don't want to be case sensitive
	shouldLower := strings.ToLower(mv.resourceQ) == mv.resourceQ
	regex, err := regexp.Compile(mv.resourceQ)
	if err != nil {
		// TODO would be nice to just do a basic string match in this case
		return err
	}
	for _, resource := range mv.allResources {
		description := resource.Description
		if shouldLower {
			description = strings.ToLower(resource.Description)
		}
		if regex.Match([]byte(description)) {
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
		source := NewLocation(jump[jumpsTable.Primary()].Format(""))
		tgt := NewLocation(jump[jumpsTable.IndexOfField(FieldTarget)].Format(""))
		description := jump[jumpsTable.Primary()].Format("")
		// NOTE a bisect would probably be more efficient here but this is good enough
		for _, res := range mv.componentLookup[source.Path] {
			if res.Location.Point > source.Point {
				break
			}
			description = res.Description
		}
		suffix := ""
		for _, res := range mv.componentLookup[tgt.Path] {
			if res.Location.Point > tgt.Point {
				break
			}
			suffix = " -> " + res.Description
		}
		description += suffix
		mv.allResources = append(mv.allResources, Resource{
			Description: description,
			Location:    source,
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
			Location:    NewLocation(component[componentsTable.IndexOfField(FieldSrcLocation)].Format("")),
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
			Location:    NewLocation(bookmark[bookmarksTable.Primary()].Format("")),
		})
	}
	return nil
}

func (mv *MainView) selectItem(g *gocui.Gui, v *gocui.View) error {
	// TODO should probably re-order these from MRU
	return mv.selectRes(g, v)
}

func (mv *MainView) selectRes(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	res := mv.activeResources[oy+cy]
	workdir, err := mv.getProjectWorkdir()
	if err != nil {
		return err
	}
	cmd := exec.Command(EMACS_CLIENT_PATH, "-n", "-s", mv.projectName, res.Location.Path)
	cmd.Dir = workdir
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command(TMUX_PATH, "send", "Escape", "x", "goto-char", "ENTER", strconv.Itoa(res.Location.Point), "ENTER")
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
	workdir, err := mv.getProjectWorkdir()
	if err != nil {
		return err
	}
	dir := filepath.Join(workdir, filepath.Dir(res.Location.Path))

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

func (mv *MainView) clearSearch(g *gocui.Gui, v *gocui.View) error {
	mv.resourceQ = ""
	return mv.refreshActiveResources()
}
