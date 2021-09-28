package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int
type ResourceType string

type Resource struct {
	Description string
	Meta        string
}

const (
	MainViewModeListResources MainViewMode = iota

	ResourceTypeCommands    ResourceType = "commands"
	ResourceTypeFrequent    ResourceType = "frequent"
	ResourceTypeRecent      ResourceType = "recent"
	ResourceTypeRecommended ResourceType = "recommended"
	ResourceTypeResources   ResourceType = "resources"
	ResourceTypeRunbooks    ResourceType = "runbooks"
)

var (
	ListResourcesTypes = []ResourceType{
		ResourceTypeCommands,
		ResourceTypeResources,
		ResourceTypeRunbooks,
	}
)

// A MainView is the overall view including a list of resources
type MainView struct {
	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	Mode   MainViewMode
	TypeIX int

	resources []Resource
	topic     string
	recursive bool
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(path string, g *gocui.Gui) (*MainView, error) {
	var store storage.Store
	if strings.HasSuffix(path, ".json") {
		store = &storage.JSONStore{}
	} else {
		return nil, fmt.Errorf("unknown file type")
	}
	mapper, err := osm.NewObjectStoreMapper(store)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		return nil, err
	}
	mv := &MainView{
		OSM:       mapper,
		DB:        db,
		topic:     "root",
		recursive: true,
	}
	return mv, mv.refreshResources()
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
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

	topics, err := g.SetView(TopicView, 0, 0, maxX-1, 2)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	topics.Clear()
	topics.Write([]byte("Topic: " + mv.topic))
	if mv.recursive {
		topics.Write([]byte(" + descendants"))
	}
	topics.Write([]byte(fmt.Sprintf(" (%d items)", len(mv.resources))))

	resources, err := g.SetView(ResourceView, 0, 4, maxX-1, maxY-4)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	resources.Highlight = true
	resources.SelBgColor = gocui.ColorWhite
	resources.SelFgColor = gocui.ColorBlack
	resources.Clear()
	for _, rec := range mv.resources {
		desc := rec.Description
		spaces := 0
		if maxX > len(desc) {
			spaces = maxX - len(desc)
		}
		resources.Write([]byte(desc + strings.Repeat(" ", spaces) + "\n"))
	}
	g.SetCurrentView(ResourceView)
	meta, err := g.SetView(MetaView, 0, maxY-3, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	meta.Clear()
	_, cy := resources.Cursor()
	_, oy := resources.Origin()
	ix := cy + oy
	if ix < len(mv.resources) {
		selected := mv.resources[ix]
		meta.Write([]byte(selected.Meta))
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
	err = g.SetKeybinding(ResourceView, 'j', gocui.ModNone, mv.incrementRec)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'k', gocui.ModNone, mv.decrementRec)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(ResourceView, gocui.KeyEnter, gocui.ModNone, mv.selectResource); err != nil {
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

func (mv *MainView) incrementRec(g *gocui.Gui, v *gocui.View) error {
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	if cy+oy == len(mv.resources)-1 {
		return nil
	}
	if err := v.SetCursor(cx, cy+1); err != nil {
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) decrementRec(g *gocui.Gui, v *gocui.View) error {
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
	mv.resources = []Resource{}
	if mv.Mode == MainViewModeListResources {
		recType := ListResourcesTypes[mv.TypeIX]
		if recType == ResourceTypeResources {
			return mv.gatherResources()
		}
	}
	return nil
}

func (mv *MainView) gatherResources() error {
	// TODO support timedb entries
	assertions := mv.DB.Tables[TableAssertions]
	nouns, err := mv.gatherFilteredNouns()
	if err != nil {
		return err
	}
	resp, err := assertions.Query(types.QueryParams{
		OrderBy: FieldArg1,
		Filters: []types.Filter{
			&nounFilter{
				col:   assertions.IndexOfField(FieldArg0),
				nouns: nouns,
			},
		},
	})
	if err != nil {
		return err
	}
	col := assertions.IndexOfField(FieldArg1)
	for _, row := range resp.Entries {
		entry := row[col].Format("")
		if !(strings.HasPrefix(entry, "[") && strings.Contains(entry, "](") && strings.HasSuffix(entry, ")")) {
			continue
		}
		parts := strings.SplitN(entry[1:len(entry)-1], "](", 2)
		mv.resources = append(mv.resources, Resource{
			Description: parts[0],
			Meta:        parts[1],
		})
	}
	return nil
}

func (mv *MainView) gatherFilteredNouns() (map[string]bool, error) {
	filtered := map[string]bool{
		mv.topic: true,
	}
	if !mv.recursive {
		return filtered, nil
	}
	node2children := map[string]([]string){}
	nouns := mv.DB.Tables[TableNouns]
	resp, err := nouns.Query(types.QueryParams{})
	if err != nil {
		return nil, err
	}
	descCol := nouns.IndexOfField(FieldNounDescription)
	parCol := nouns.IndexOfField(FieldParent)
	for _, row := range resp.Entries {
		desc := row[descCol].Format("")
		parent := row[parCol].Format("")
		node2children[parent] = append(node2children[parent], desc)
	}
	queue := node2children[mv.topic]
	for {
		if len(queue) == 0 {
			break
		}
		node := queue[0]
		queue = queue[1:]
		if filtered[node] {
			return nil, errors.New("cycle detected")
		}
		filtered[node] = true
		queue = append(queue, node2children[node]...)
	}
	return filtered, nil
}

func (mv *MainView) selectResource(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	resource := mv.resources[ix]
	if mv.Mode == MainViewModeListResources {
		recType := ListResourcesTypes[mv.TypeIX]
		if recType == ResourceTypeResources {
			link := resource.Meta
			cmd := exec.Command("txtopen", link)
			return cmd.Run()
		}
	}
	return nil
}
