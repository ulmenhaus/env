package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
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
	Params      map[string]string
	ShortDesc   string
}

const (
	MainViewModeListResources MainViewMode = iota
	MainViewModeQueryResources
	MainViewModeFilterResources
	MainViewModeSearchTopics

	ResourceTypeCommands    ResourceType = "commands"
	ResourceTypeFrequent    ResourceType = "frequent"
	ResourceTypeRecent      ResourceType = "recent"
	ResourceTypeRecommended ResourceType = "recommended"
	ResourceTypeResources   ResourceType = "resources"
	ResourceTypeProcedures  ResourceType = "procedures"
)

var (
	ListResourcesTypes = []ResourceType{
		ResourceTypeCommands,
		ResourceTypeResources,
		ResourceTypeProcedures,
	}
)

// A MainView is the overall view including a list of resources
type MainView struct {
	OSM       *osm.ObjectStoreMapper
	DB        *types.Database
	jqlBinDir string

	Mode   MainViewMode
	TypeIX int

	resources []Resource
	resourceQ string
	topic     string
	topics    []string
	recursive bool
	topicQ    string
	selected  map[string](map[string]bool) // maps key to the possible values it can take
	filters   [][]string
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(path string, g *gocui.Gui, jqlBinDir, defaultResourceFilter string) (*MainView, error) {
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
	rootTopic := RootTopic
	if defaultResourceFilter != "" {
		rootTopic = defaultResourceFilter
	}
	mv := &MainView{
		OSM:       mapper,
		DB:        db,
		topic:     rootTopic,
		TypeIX:    1,
		recursive: true,
		jqlBinDir: jqlBinDir,
	}
	return mv, mv.refreshResources()
}

// Edit handles keyboard inputs while searching
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if mv.Mode == MainViewModeSearchTopics {
		mv.editSearch(v, key, ch, mod)
		return
	} else if mv.Mode == MainViewModeQueryResources {
		mv.editQuery(v, key, ch, mod)
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
	if mv.Mode == MainViewModeListResources || mv.Mode == MainViewModeQueryResources {
		return mv.listResourcesLayout(g)
	} else if mv.Mode == MainViewModeSearchTopics {
		return mv.searchTopicsLayout(g)
	} else if mv.Mode == MainViewModeFilterResources {
		return mv.filterResourcesLayout(g)
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
		var buffer string
		spaces := maxX - len(topic)
		if spaces > 0 {
			buffer = strings.Repeat(" ", spaces)
		}
		topics.Write([]byte(topic + buffer + "\n"))
	}
	query, err := g.SetView(TopicsQueryView, 4, maxY-7, maxX-4, maxY-5)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	query.Clear()
	query.Write([]byte(mv.topicQ))
	return nil
}

func (mv *MainView) filterResourcesLayout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	filters, err := g.SetView(FilterView, 4, 5, maxX-4, maxY-7)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	filters.Highlight = true
	filters.SelBgColor = gocui.ColorWhite
	filters.SelFgColor = gocui.ColorBlack
	filters.Editable = true
	filters.Editor = mv
	filters.Clear()
	g.SetCurrentView(FilterView)
	for _, filter := range mv.filters {
		if len(filter) == 1 {
			key := filter[0]
			filters.Write([]byte(fmt.Sprintf("%s\n", key)))
		} else if len(filter) == 2 {
			key, val := filter[0], filter[1]
			selected := mv.selected[key][val]
			box := "[ ]"
			if selected {
				box = "[x]"
			}
			filters.Write([]byte(fmt.Sprintf("  %s %s\n", box, val)))
		}
	}
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
	if mv.Mode == MainViewModeListResources {
		if ix < len(mv.resources) {
			selected := mv.resources[ix]
			meta.Write([]byte(selected.Meta))
		}
		meta.Editable = false
		if mv.resourceQ != "" {
			topic.Write([]byte("    Query: " + mv.resourceQ))
		}
	} else if mv.Mode == MainViewModeQueryResources {
		meta.Write([]byte(mv.resourceQ))
		meta.Editable = true
		meta.Editor = mv
		g.SetCurrentView(MetaView)
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
	err = g.SetKeybinding(ResourceView, 'd', gocui.ModNone, mv.enterFilterMode)
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
	err = g.SetKeybinding(ResourceView, '/', gocui.ModNone, mv.toggleSearch)
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
	err = g.SetKeybinding(FilterView, 'j', gocui.ModNone, mv.incrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(FilterView, 'k', gocui.ModNone, mv.decrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(FilterView, 'q', gocui.ModNone, mv.quitFilterView)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ResourceView, 'C', gocui.ModNone, mv.copyAllProcedures)
	if err != nil {
		return err
	}
	if err := g.SetKeybinding(FilterView, gocui.KeyEnter, gocui.ModNone, mv.selectFilter); err != nil {
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
	if mv.Mode == MainViewModeListResources || mv.Mode == MainViewModeQueryResources {
		err := mv.gatherResources()
		if err != nil {
			return err
		}
		mv.resetFilters()
		return nil
	}
	return nil
}

func (mv *MainView) resetFilters() {
	mv.selected = map[string](map[string]bool){}
	for _, resource := range mv.resources {
		for key, val := range resource.Params {
			if _, ok := mv.selected[key]; !ok {
				mv.selected[key] = map[string]bool{
					"-none set": true,
				}
			}
			mv.selected[key][val] = true
		}
	}
	mv.filters = [][]string{}
	keys := make([]string, 0, len(mv.selected))
	for key := range mv.selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		mv.filters = append(mv.filters, []string{key})
		vals := make([]string, 0, len(mv.selected[key]))
		for val := range mv.selected[key] {
			vals = append(vals, val)
		}
		sort.Strings(vals)
		for _, val := range vals {
			mv.filters = append(mv.filters, []string{key, val})
		}
	}
}

func (mv *MainView) gatherResources() error {
	mv.resources = []Resource{}
	// TODO support timedb entries
	assertions := mv.DB.Tables[TableAssertions]
	nouns, err := mv.gatherFilteredNouns(mv.recursive)
	if err != nil {
		return err
	}
	rel := assertions.IndexOfField(FieldRelation)
	arg0 := assertions.IndexOfField(FieldArg0)
	arg1 := assertions.IndexOfField(FieldArg1)
	recType := ListResourcesTypes[mv.TypeIX]
	var filter types.Filter
	if recType == ResourceTypeResources {
		filter = &eqFilter{
			col: rel,
			val: ".Resource",
		}
	} else if recType == ResourceTypeCommands {
		filter = &eqFilter{
			col: rel,
			val: ".Command",
		}
	} else if recType == ResourceTypeProcedures {
		filter = &eqFilter{
			col: rel,
			val: ".Procedure",
		}
	}
	resp, err := assertions.Query(types.QueryParams{
		OrderBy: FieldArg1,
		Filters: []types.Filter{
			&nounFilter{
				col:   arg0,
				nouns: nouns,
			},
			// TODO since we include arg0 in procedures this search can give false negatives
			&inFilter{
				col: arg1,
				val: mv.resourceQ,
			},
			filter,
		},
	})
	if err != nil {
		return err
	}
	for _, row := range resp.Entries {
		entry := row[arg1].Format("")
		if recType == ResourceTypeResources {
			if !(strings.HasPrefix(entry, "[") && strings.Contains(entry, "](") && strings.HasSuffix(entry, ")")) {
				continue
			}
			parts := strings.SplitN(entry[1:len(entry)-1], "](", 2)
			mv.resources = append(mv.resources, Resource{
				Description: parts[0],
				Meta:        parts[1],
			})
		} else if recType == ResourceTypeCommands {
			lines := strings.Split(entry, "\n")
			if !(len(lines) > 2 && strings.HasPrefix(lines[0], "### ")) {
				continue
			}
			if strings.HasPrefix(lines[1], "```") {
				// Singleton command
				mv.resources = append(mv.resources, Resource{
					Description: lines[0][len("### "):],
					Meta:        strings.Replace(lines[2], "\\|", "|", -1),
				})
			} else {
				// Look for table entries
				count := 0
				topic := lines[0][len("### "):]
				for _, line := range lines {
					if !strings.HasPrefix(line, "| ") {
						continue
					}
					count += 1
					if count < 2 {
						continue
					}
					command := strings.Replace(strings.Split(line, "`")[1], "\\|", "|", -1)
					description := strings.Split(line, "|")[1][1:]
					mv.resources = append(mv.resources, Resource{
						Description: description,
						Meta:        command,
						Params:      map[string]string{"topic": topic},
					})
				}
			}
		} else if recType == ResourceTypeProcedures {
			if !strings.HasPrefix(entry, "### ") {
				continue
			}
			lines := strings.Split(entry, "\n")
			content := ""
			if len(lines) > 1 {
				content = strings.Join(lines[1:], "\n")
			}
			parts := strings.Split(lines[0], " ")
			verb := parts[1]
			indirect := ""
			indirectExp := ""
			inparens := []string{}
			params := map[string]string{}
			if len(parts) > 2 {
				for _, param := range parts[2:] {
					if !strings.Contains(param, "=") {
						continue
					}
					paramParts := strings.SplitN(param, "=", 2)
					key, val := paramParts[0], paramParts[1]
					params[key] = val
					if key == "subset" {
						indirect = val
						indirectExp = " with " + val
					} else {
						inparens = append(inparens, param)
					}
				}
			}
			paramsS := ""
			if len(inparens) > 0 {
				paramsS = fmt.Sprintf(" (%s)", strings.Join(inparens, " "))
			}
			shortDescComponents := inparens
			if indirect != "" {
				shortDescComponents = append([]string{strings.Title(indirect)}, shortDescComponents...)
			}
			object := strings.SplitN(row[arg0].Format(""), " ", 2)[1]
			mv.resources = append(mv.resources, Resource{
				Description: fmt.Sprintf("%s %s%s%s", verb, object, indirectExp, paramsS),
				Meta:        content,
				Params:      params,
				ShortDesc:   strings.Join(shortDescComponents, " "),
			})
		}
	}
	sort.Slice(mv.resources, func(i, j int) bool {
		return mv.resources[i].Description < mv.resources[j].Description
	})
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
	descCol := nouns.IndexOfField(FieldNounIdentifier)
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
			return nil, errors.New(fmt.Sprintf("cycle detected from: %s", node))
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
		cmd := exec.Command(path.Join(mv.jqlBinDir, "txtopen"), link)
		err := cmd.Run()
		if err != nil {
			return err
		}
		os.Exit(0)
		return nil
	}
	if recType == ResourceTypeCommands {
		command := resource.Meta
		// XXX hard-coding the tmux path is not portable
		cmd := exec.Command("/usr/local/bin/tmux", "send", "--", command)
		err := cmd.Run()
		if err != nil {
			return err
		}
		os.Exit(0)
		return nil
	}
	if recType == ResourceTypeProcedures {
		// XXX hard-coding the tmux and runproc path is not portable
		cmd := exec.Command("/usr/local/bin/tmux", "split-window", "-fb", "-l", "15", "--", "/usr/local/bin/runproc", resource.Description, resource.Meta)
		err := cmd.Run()
		if err != nil {
			return err
		}
		os.Exit(0)
		return nil
	}
	return nil
}

func (mv *MainView) selectFilter(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	filter := mv.filters[ix]
	if len(filter) == 1 {
		key := filter[0]
		allSelected := true
		for _, selected := range mv.selected[key] {
			if !selected {
				allSelected = false
			}
		}
		shouldSelect := !allSelected
		for val := range mv.selected[key] {
			mv.selected[key][val] = shouldSelect
		}
	} else if len(filter) == 2 {
		key, val := filter[0], filter[1]
		mv.selected[key][val] = !mv.selected[key][val]
	}
	return nil
}

func (mv *MainView) enterSearchMode(g *gocui.Gui, v *gocui.View) error {
	mv.Mode = MainViewModeSearchTopics
	return mv.setTopics()
}

func (mv *MainView) enterFilterMode(g *gocui.Gui, v *gocui.View) error {
	if ListResourcesTypes[mv.TypeIX] != ResourceTypeProcedures && ListResourcesTypes[mv.TypeIX] != ResourceTypeCommands {
		return nil
	}
	mv.Mode = MainViewModeFilterResources
	return nil
}

func (mv *MainView) restoreDefault(g *gocui.Gui, v *gocui.View) error {
	mv.topic = RootTopic
	mv.resourceQ = ""
	return mv.refreshResources()
}

func (mv *MainView) toggleRecursive(g *gocui.Gui, v *gocui.View) error {
	mv.recursive = !mv.recursive
	return mv.refreshResources()
}

func (mv *MainView) toggleSearch(g *gocui.Gui, v *gocui.View) error {
	mv.Mode = MainViewModeQueryResources
	return nil
}

func (mv *MainView) quitFilterView(g *gocui.Gui, v *gocui.View) error {
	err := g.DeleteView(FilterView)
	if err != nil {
		return err
	}
	mv.Mode = MainViewModeListResources
	return mv.filterResources()
}

func (mv *MainView) filterResources() error {
	err := mv.gatherResources()
	if err != nil {
		return nil
	}
	resources := mv.resources
	mv.resources = []Resource{}
	for _, resource := range resources {
		visible := true
		for key := range mv.selected {
			val := "-none set"
			if actualVal, ok := resource.Params[key]; ok {
				val = actualVal
			}
			if !mv.selected[key][val] {
				visible = false
			}
		}
		if visible {
			mv.resources = append(mv.resources, resource)
		}
	}
	return nil
}

func (mv *MainView) copyAllProcedures(g *gocui.Gui, v *gocui.View) error {
	if ListResourcesTypes[mv.TypeIX] != ResourceTypeProcedures {
		return nil
	}
	content := ""
	for _, resource := range mv.resources {
		content += "### " + resource.ShortDesc + "\n\n"
		steps := strings.Split(resource.Meta, "\n- ")
		for _, step := range steps {
			if step == "" {
				continue
			}
			content += "- [ ] " + step + "\n"
		}
	}
	cmd := exec.Command(path.Join(mv.jqlBinDir, "txtcopy"))
	cmd.Stdin = strings.NewReader(content)
	err := cmd.Run()
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
