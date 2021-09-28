package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
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
	MainViewModeSearchTopics

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
	topics    []string
	recursive bool
	topicQ    string
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
		topic:     RootTopic,
		recursive: true,
	}
	return mv, mv.refreshResources()
}

// Edit handles keyboard inputs while searching
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if mv.Mode == MainViewModeSearchTopics {
		mv.editSearch(v, key, ch, mod)
		return
	}
}

func (mv *MainView) editSearch(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(mv.topicQ) != 0 {
			mv.topicQ = mv.topicQ[:len(mv.topicQ)-1]
		}
	} else if key == gocui.KeySpace {
		mv.topicQ += " "
	} else {
		mv.topicQ += string(ch)
	}
	mv.setTopics()
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	if mv.Mode == MainViewModeListResources {
		return mv.listResourcesLayout(g)
	} else if mv.Mode == MainViewModeSearchTopics {
		return mv.searchTopicsLayout(g)
	}
	return nil
}

func (mv *MainView) setTopics() error {
	nouns, err := mv.gatherFilteredNouns(true)
	if err != nil {
		return err
	}
	slice := []string{}
	for noun := range nouns {
		if strings.Contains(strings.ToLower(noun), strings.ToLower(mv.topicQ)) {
			slice = append(slice, noun)
		}
	}
	sorted := sort.StringSlice(slice)
	sorted.Sort()
	mv.topics = []string(sorted)
	return nil
}

func (mv *MainView) searchTopicsLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	topics, err := g.SetView(TopicsView, 4, 5, maxX-4, maxY-7)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	topics.Highlight = true
	topics.SelBgColor = gocui.ColorWhite
	topics.SelFgColor = gocui.ColorBlack
	topics.Editable = true
	topics.Editor = mv
	topics.Clear()
	g.SetCurrentView(TopicsView)
	for _, topic := range mv.topics {
		spaces := maxX - len(topic)
		topics.Write([]byte(topic + strings.Repeat(" ", spaces) + "\n"))
	}
	query, err := g.SetView(TopicsQueryView, 4, maxY-7, maxX-4, maxY-5)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	query.Clear()
	query.Write([]byte(mv.topicQ))
	return nil
}

func (mv *MainView) listResourcesLayout(g *gocui.Gui) error {
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

	topic, err := g.SetView(TopicView, 0, 0, maxX-1, 2)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	topic.Clear()
	topic.Write([]byte("Topic: " + mv.topic))
	if mv.recursive {
		topic.Write([]byte(" + descendants"))
	}
	topic.Write([]byte(fmt.Sprintf(" (%d items)", len(mv.resources))))

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
	err = g.SetKeybinding(ResourceView, 'q', gocui.ModNone, mv.restoreDefault)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'r', gocui.ModNone, mv.toggleRecursive)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TopicsView, gocui.KeyArrowDown, gocui.ModNone, mv.incrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(TopicsView, gocui.KeyArrowUp, gocui.ModNone, mv.decrementCursor)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(ResourceView, gocui.KeyEnter, gocui.ModNone, mv.selectItem); err != nil {
		return err
	}
	if err := g.SetKeybinding(TopicsView, gocui.KeyEnter, gocui.ModNone, mv.selectItem); err != nil {
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
	nouns, err := mv.gatherFilteredNouns(mv.recursive)
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

func (mv *MainView) gatherFilteredNouns(recursive bool) (map[string]bool, error) {
	filtered := map[string]bool{
		mv.topic: true,
	}
	if !recursive {
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

func (mv *MainView) selectItem(g *gocui.Gui, v *gocui.View) error {
	if mv.Mode == MainViewModeListResources {
		return mv.selectResource(g, v)
	} else if mv.Mode == MainViewModeSearchTopics {
		return mv.selectTopic(g)
	}
	return nil
}

func (mv *MainView) selectTopic(g *gocui.Gui) error {
	v, err := g.View(TopicsView)
	if err != nil {
		return err
	}
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	topic := mv.topics[ix]
	mv.topic = topic
	err = g.DeleteView(TopicsView)
	if err != nil {
		return err
	}
	err = g.DeleteView(TopicsQueryView)
	if err != nil {
		return err
	}
	main, err := g.View(ResourceView)
	if err != nil {
		return err
	}
	main.Editable = false
	// NOTE should store previous value in case view is secondary view (when implemented)
	mv.Mode = MainViewModeListResources
	mv.topicQ = ""
	return mv.refreshResources()
}

func (mv *MainView) selectResource(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	resource := mv.resources[ix]
	recType := ListResourcesTypes[mv.TypeIX]
	if recType == ResourceTypeResources {
		link := resource.Meta
		cmd := exec.Command("txtopen", link)
		return cmd.Run()
	}
	return nil
}

func (mv *MainView) enterSearchMode(g *gocui.Gui, v *gocui.View) error {
	mv.Mode = MainViewModeSearchTopics
	return mv.setTopics()
}

func (mv *MainView) restoreDefault(g *gocui.Gui, v *gocui.View) error {
	mv.topic = RootTopic
	return mv.refreshResources()
}

func (mv *MainView) toggleRecursive(g *gocui.Gui, v *gocui.View) error {
	mv.recursive = !mv.recursive
	return mv.refreshResources()
}
