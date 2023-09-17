package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/img/jql/ui"
	"golang.org/x/sync/errgroup"
)

const (
	blackTextEscape = "\033[30m"
	whiteBackEscape = "\033[47m"
	boldTextEscape  = "\033[1m"
	resetEscape     = "\033[0m"
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int

const (
	MainViewModeListBar MainViewMode = iota
)

// A MainView is the overall view including a resource list
// and a detailed view of the current resource
type MainView struct {
	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	Mode MainViewMode

	path string

	ignored     map[string](map[string]bool) // stores a map from feed name to a set of ignored entries
	ignoredPath string

	selectedDomain int
	domains        []*domain
	name2channel   map[string]*channel
}

type domain struct {
	name     string
	channels []string
}

type channel struct {
	row          []types.Entry
	status2items map[string][]*Item
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
		OSM: mapper,
		DB:  db,

		path: path,

		name2channel: map[string]*channel{},
	}
	mv.ignoredPath = path + ".ignored"
	_, err = os.Stat(mv.ignoredPath)
	if os.IsNotExist(err) {
		mv.ignored = map[string](map[string]bool){}
	} else if err != nil {
		return nil, err
	} else {
		ignored := map[string](map[string]bool){}
		contents, err := os.ReadFile(mv.ignoredPath)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(contents, &ignored)
		if err != nil {
			return nil, err
		}
		mv.ignored = ignored
	}
	err = mv.fetchResources()
	if err != nil {
		return nil, err
	}
	return mv, mv.refreshView(g)
}

func (mv *MainView) gatherDomains() (map[string]string, error) {
	assnTable, ok := mv.DB.Tables[TableAssertions]
	if !ok {
		return nil, fmt.Errorf("expected assertions table to exist")
	}
	resp, err := assnTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldRelation,
				Col:       assnTable.IndexOfField(FieldRelation),
				Formatted: ".Domain",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	noun2domain := map[string]string{}
	for _, entry := range resp.Entries {
		arg0 := entry[assnTable.IndexOfField(FieldArg0)].Format("")
		arg1 := entry[assnTable.IndexOfField(FieldArg1)].Format("")
		if !strings.HasPrefix(arg0, "nouns ") {
			continue
		}
		if !strings.HasPrefix(arg1, "@timedb:") {
			continue
		}
		if !strings.HasSuffix(arg1, ":") {
			continue
		}
		source := arg0[len("nouns "):]
		dst := arg1[len("@timedb:") : len(arg1)-1]
		noun2domain[source] = dst
	}
	return noun2domain, nil
}

func (mv *MainView) fetchResources() error {
	// TODO use constants for column names
	// TODO would be good to parallelize fetches
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected nouns table to exist")
	}
	resp, err := nounsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldFeed,
				Col:       nounsTable.IndexOfField(FieldFeed),
				Formatted: "",
				Not:       true,
			},
		},
		OrderBy: FieldIdentifier,
	})
	if err != nil {
		return err
	}
	noun2domain, err := mv.gatherDomains()
	if err != nil {
		return err
	}
	domains := map[string]*domain{}
	/*
		Should only add resources if they're active and have someday or idea
		status. If they have neither and no in progress tasks and no feed, then we also
		add it.
	*/
	for _, entry := range resp.Entries {
		feed := entry[nounsTable.IndexOfField(FieldFeed)].Format("")
		if (!strings.Contains(feed, "://")) && feed != "manual" {
			continue
		}
		entryName := entry[nounsTable.IndexOfField(FieldIdentifier)].Format("")
		domainName := noun2domain[entryName]
		if domainName == "" {
			domainName = "Attention Domains"
		}
		if _, ok := domains[domainName]; !ok {
			domains[domainName] = &domain{name: domainName}
		}
		domain := domains[domainName]
		name := entry[nounsTable.IndexOfField(FieldIdentifier)].Format("")
		mv.name2channel[name] = &channel{
			row:          entry,
			status2items: map[string][]*Item{},
		}
		domain.channels = append(domain.channels, name)
		if mv.ignored[entryName] == nil {
			mv.ignored[entryName] = map[string]bool{}
		}
	}
	for _, domain := range domains {
		mv.domains = append(mv.domains, domain)
	}
	sort.Slice(mv.domains, func(i, j int) bool {
		iName, jName := mv.domains[i].name, mv.domains[j].name
		// We want to show generic Attention Domains last
		return (iName < jName && iName != "Attention Domains") || jName == "Attention Domains"
	})

	return nil
}

func (mv *MainView) fetchNewItems(g *gocui.Gui, v *gocui.View) error {
	// TODO worth taking a second pass at this function for code cleanliness and performance
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected nouns table to exist")
	}
	contextTable, ok := mv.DB.Tables[TableContexts]
	if !ok {
		return fmt.Errorf("expected context table to exist")
	}

	allContexts, err := contextTable.Query(types.QueryParams{})
	if err != nil {
		return err
	}
	parent2context := map[string]string{}
	for _, entry := range allContexts.Entries {
		parent2context[entry[contextTable.IndexOfField(FieldParent)].Format("")] = entry[contextTable.IndexOfField(FieldCode)].Format("")
	}

	allItems, err := nounsTable.Query(types.QueryParams{})
	if err != nil {
		return err
	}
	allIDs := map[string]bool{}
	for _, rawItem := range allItems.Entries {
		allIDs[rawItem[nounsTable.IndexOfField(FieldIdentifier)].Format("")] = true
	}
	group := new(errgroup.Group)
	semaphore := make(chan bool, 5) // Limit parallel requests
	for _, domain := range mv.domains {
		for _, name := range domain.channels {
			entry := mv.name2channel[name]
			entryName := entry.row[nounsTable.IndexOfField(FieldIdentifier)].Format("")
			channel := mv.name2channel[entryName]
			channel.status2items[FreshView] = nil

			feedURL := entry.row[nounsTable.IndexOfField(FieldFeed)].Format("")
			if !strings.Contains(feedURL, "://") {
				continue
			}
			feed, err := NewFeed(feedURL, parent2context[entryName])
			if err != nil {
				return err
			}
			group.Go(func() error {
				semaphore <- true
				defer func() { <-semaphore }()
				latest, err := feed.FetchNew()
				if err != nil {
					return fmt.Errorf("Failed to fetch feed for %s: %s", entry.row[nounsTable.IndexOfField(FieldIdentifier)].Format(""), err)
				}
				for _, item := range latest {
					if !(allIDs[item.Identifier] || mv.ignored[entryName][item.Identifier]) {
						channel.status2items[FreshView] = append(channel.status2items[FreshView], item)
					}
				}
				return nil
			})
		}
	}
	return group.Wait()
}

func (mv *MainView) addFreshItem(g *gocui.Gui, v *gocui.View) error {
	resources, err := g.View(ResourcesView)
	if err != nil {
		return err
	}
	_, selectedResource := resources.Cursor()
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected resources table to exist")
	}
	feed := mv.name2channel[mv.domains[mv.selectedDomain].channels[selectedResource]]
	entryName := feed.row[nounsTable.IndexOfField(FieldIdentifier)].Format("")
	_, cy := v.Cursor()
	_, oy := v.Origin()
	item := mv.name2channel[entryName].status2items[FreshView][oy+cy]
	identifier := item.Identifier
	err = nounsTable.Insert(identifier)
	if err != nil {
		// TODO would be good to use a specific error type
		if strings.HasPrefix(err.Error(), "Row already exists") {
			for i := 1; i < 100; i++ {
				identifier = fmt.Sprintf("%s (%02d)", item.Identifier, i)
				err = nounsTable.Insert(identifier)
				if err == nil {
					break
				} else if strings.HasPrefix(err.Error(), "Row already exists") {
					continue
				} else {
					return err
				}
			}
		} else {
			return fmt.Errorf("Failed to add entry: %s", err)
		}
	}

	// TODO now that we support insert with fields we probably don't need separate updates here
	err = nounsTable.Update(identifier, FieldLink, item.Link)
	if err != nil {
		return fmt.Errorf("Failed to update link for entry: %s", err)
	}

	err = nounsTable.Update(identifier, FieldParent, entryName)
	if err != nil {
		return fmt.Errorf("Failed to update resource for entry: %s", err)
	}
	err = nounsTable.Update(identifier, FieldDescription, item.Description)
	if err != nil {
		return fmt.Errorf("Failed to update description for entry: %s", err)
	}
	err = nounsTable.Update(identifier, FieldContext, item.Context)
	if err != nil {
		return fmt.Errorf("Failed to update context for entry: %s", err)
	}
	channel := mv.name2channel[entryName]
	channel.status2items[FreshView] = append(channel.status2items[FreshView][:oy+cy], channel.status2items[FreshView][oy+cy+1:]...)
	return mv.refreshView(g)
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) layoutDomains(g *gocui.Gui, domainHeight int) error {
	maxX, _ := g.Size()
	domains, err := g.SetView(DomainView, 0, 0, maxX-1, domainHeight)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	domains.Clear()
	domainWidth := maxX / len(mv.domains)
	for i, domain := range mv.domains {
		name := "  " + domain.name
		totalFresh := 0
		for _, channel := range domain.channels {
			totalFresh += len(mv.name2channel[channel].status2items[FreshView])
		}
		lenCorrection := 0
		if totalFresh > 0 {
			name += fmt.Sprintf(" %s(%d)%s", boldTextEscape, totalFresh, resetEscape)
			lenCorrection = -10
		}
		if (len(name) + lenCorrection) < domainWidth {
			buffer := strings.Repeat(" ", (domainWidth-(len(name)+lenCorrection))/2)
			name = buffer + name + buffer
		}
		if i == mv.selectedDomain {
			domains.Write([]byte(blackTextEscape + whiteBackEscape))
		}
		domains.Write([]byte(name))
		if i == mv.selectedDomain {
			domains.Write([]byte(resetEscape))
		}
	}
	return nil
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected nouns table to exist")
	}
	maxX, maxY := g.Size()
	domainHeight := 2
	if err := mv.layoutDomains(g, domainHeight); err != nil {
		return err
	}
	resourcesWidth := 40
	pipeHeight := maxY / 4
	pipeOffset := func(i int) int {
		return domainHeight + 1 + pipeHeight*i
	}
	active, err := g.SetView(StatusActive, resourcesWidth+1, pipeOffset(0), maxX-1, pipeOffset(0)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	pending, err := g.SetView(StatusPending, resourcesWidth+1, pipeOffset(1), maxX-1, pipeOffset(1)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	idea, err := g.SetView(StatusIdea, resourcesWidth+1, pipeOffset(2), maxX-1, pipeOffset(2)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	fresh, err := g.SetView(FreshView, resourcesWidth+1, pipeOffset(3), maxX-1, pipeOffset(3)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	resources, err := g.SetView(ResourcesView, 0, domainHeight+1, resourcesWidth, maxY-1)
	if err != nil && err == gocui.ErrUnknownView {
		_, err = g.SetCurrentView(ResourcesView)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	resources.Clear()
	for _, view := range []*gocui.View{active, pending, idea, fresh, resources} {
		view.SelBgColor = gocui.ColorWhite
		view.SelFgColor = gocui.ColorBlack
		view.Highlight = view == g.CurrentView()
		view.Clear()
	}

	for _, name := range mv.domains[mv.selectedDomain].channels {
		channel := mv.name2channel[name]
		description := channel.row[nounsTable.IndexOfField(FieldDescription)].Format("")
		if len(channel.status2items[FreshView]) > 0 {
			description += fmt.Sprintf(" %s(%d)%s", boldTextEscape, len(channel.status2items[FreshView]), resetEscape)
		}
		fmt.Fprintf(resources, "  %s\n", description)
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	for status, items := range channel.status2items {
		for _, item := range items {
			switch status {
			case FreshView, StatusActive, StatusPending, StatusIdea:
				view, err := g.View(status)
				if err != nil {
					return err
				}
				padding := item.Coordinal
				if padding == "" {
					padding = "   "
				}
				fmt.Fprintf(view, "  %s\t%s\n", padding, item.Description)
			}
		}
	}
	return nil
}

func (mv *MainView) saveContents(g *gocui.Gui, v *gocui.View) error {
	return mv.save()
}

func (mv *MainView) save() error {
	f, err := os.OpenFile(mv.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	err = mv.OSM.Dump(mv.DB, f)
	if err != nil {
		return err
	}
	serialized, err := json.MarshalIndent(mv.ignored, "", "    ")
	if err != nil {
		return err
	}
	err = os.WriteFile(mv.ignoredPath, serialized, 0600)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	nextMap := map[string]string{
		ResourcesView: FreshView,
		FreshView:     StatusIdea,
		StatusIdea:    StatusPending,
		StatusPending: StatusActive,
		StatusActive:  ResourcesView,
	}
	for current, next := range nextMap {
		err := g.SetKeybinding(current, 'f', gocui.ModNone, mv.fetchNewItems)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'n', gocui.ModNone, mv.switcherTo(next))
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'q', gocui.ModNone, mv.switchToJQL)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'g', gocui.ModNone, mv.goToPK)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(next, 'N', gocui.ModNone, mv.switcherTo(current))
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'j', gocui.ModNone, mv.cursorDown)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'k', gocui.ModNone, mv.cursorUp)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 's', gocui.ModNone, mv.saveContents)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'l', gocui.ModNone, mv.incrementDomain)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'h', gocui.ModNone, mv.decrementDomain)
		if err != nil {
			return err
		}
		if current == ResourcesView {
			continue
		}
		err = g.SetKeybinding(current, 'i', gocui.ModNone, mv.moveUp)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'I', gocui.ModNone, mv.moveDown)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'J', gocui.ModNone, mv.moveDownInPipe)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'K', gocui.ModNone, mv.moveUpInPipe)
		if err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) switcherTo(name string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		_, err := g.SetCurrentView(name)
		return err
	}
}

func (mv *MainView) moveDownInPipe(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == ResourcesView || name == FreshView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("Expected to find nouns table")
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	pk := channel.status2items[name][oy+cy].Identifier
	new, err := nounsTable.Entries[pk][nounsTable.IndexOfField(FieldCoordinal)].Add(1)
	if err != nil {
		return err
	}
	nounsTable.Entries[pk][nounsTable.IndexOfField(FieldCoordinal)] = new
	if oy+cy+1 < len(channel.status2items[name]) {
		successor := channel.status2items[name][oy+cy+1].Identifier
		new, err := nounsTable.Entries[successor][nounsTable.IndexOfField(FieldCoordinal)].Add(-1)
		if err != nil {
			return err
		}
		nounsTable.Entries[successor][nounsTable.IndexOfField(FieldCoordinal)] = new
	}
	return mv.cursorDown(g, v)
}

func (mv *MainView) moveUpInPipe(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == ResourcesView || name == FreshView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("Expected to find nouns table")
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	pk := channel.status2items[name][oy+cy].Identifier
	new, err := nounsTable.Entries[pk][nounsTable.IndexOfField(FieldCoordinal)].Add(-1)
	if err != nil {
		return err
	}
	nounsTable.Entries[pk][nounsTable.IndexOfField(FieldCoordinal)] = new
	if oy+cy-1 >= 0 {
		predecessor := channel.status2items[name][oy+cy-1].Identifier
		new, err := nounsTable.Entries[predecessor][nounsTable.IndexOfField(FieldCoordinal)].Add(1)
		if err != nil {
			return err
		}
		nounsTable.Entries[predecessor][nounsTable.IndexOfField(FieldCoordinal)] = new
	}
	return mv.cursorUp(g, v)
}

func (mv *MainView) moveDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == ResourcesView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("Expected to find nouns table")
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	pk := channel.status2items[name][oy+cy].Identifier
	if name == FreshView {
		resources, err := g.View(ResourcesView)
		if err != nil {
			return err
		}
		_, roy := resources.Origin()
		_, rcy := resources.Cursor()
		entry := mv.name2channel[mv.domains[mv.selectedDomain].channels[roy+rcy]].row
		entryName := entry[nounsTable.IndexOfField(FieldIdentifier)].Format("")
		mv.ignored[entryName][pk] = true
		channel := mv.name2channel[entryName]
		channel.status2items[FreshView] = append(channel.status2items[FreshView][:oy+cy], channel.status2items[FreshView][oy+cy+1:]...)
	} else {
		new, err := nounsTable.Entries[pk][nounsTable.IndexOfField(FieldStatus)].Add(-1)
		if err != nil {
			return err
		}
		nounsTable.Entries[pk][nounsTable.IndexOfField(FieldStatus)] = new
	}
	return mv.refreshView(g)
}

func (mv *MainView) incrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + 1) % len(mv.domains)
	return mv.refreshView(g)
}

func (mv *MainView) decrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + len(mv.domains) - 1) % len(mv.domains)
	return mv.refreshView(g)
}

func (mv *MainView) moveUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == ResourcesView {
		return nil
	}
	if name == FreshView {
		return mv.addFreshItem(g, v)
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	pk := channel.status2items[name][oy+cy].Identifier
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("Expected to find items table")
	}
	new, err := nounsTable.Entries[pk][nounsTable.IndexOfField(FieldStatus)].Add(1)
	if err != nil {
		return err
	}
	nounsTable.Entries[pk][nounsTable.IndexOfField(FieldStatus)] = new
	return mv.refreshView(g)
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy+1); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	ox, oy := v.Origin()
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) goToJQL(extraArgs ...string) error {
	err := mv.save()
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(JQLName)
	if err != nil {
		return err
	}

	args := []string{JQLName, mv.path}
	args = append(args, extraArgs...)

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) goToPK(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	var pk string
	if v.Name() == ResourcesView {
		nounsTable, ok := mv.DB.Tables[TableNouns]
		if !ok {
			return fmt.Errorf("expected nouns table to exist")
		}
		pk = mv.name2channel[mv.domains[mv.selectedDomain].channels[oy+cy]].row[nounsTable.IndexOfField(FieldIdentifier)].Format("")
	} else {
		channel, err := mv.selectedChannel(g)
		if err != nil {
			return err
		}
		pk = channel.status2items[v.Name()][oy+cy].Identifier
	}
	return mv.goToJQL(TableNouns, pk)
}

func (mv *MainView) switchToJQL(g *gocui.Gui, v *gocui.View) error {
	return mv.goToJQL(TableNouns)
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	// TODO this method could use a second pass for code cleanliness and performance
	nounsTable, ok := mv.DB.Tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected nouns table to exist")
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	entryName := channel.row[nounsTable.IndexOfField(FieldIdentifier)].Format("")

	for _, status := range []string{StatusActive, StatusPending, StatusIdea} {
		channel.status2items[status] = nil
	}
	rawItems, err := nounsTable.Query(types.QueryParams{
		Filters: []types.Filter{
			&ui.EqualFilter{
				Field:     FieldParent,
				Col:       nounsTable.IndexOfField(FieldParent),
				Formatted: entryName,
			},
		},
	})
	if err != nil {
		return err
	}

	for _, rawItem := range rawItems.Entries {
		status := rawItem[nounsTable.IndexOfField(FieldStatus)].Format("")
		if channel.status2items[status] == nil {
			channel.status2items[status] = []*Item{}
		}
		channel.status2items[status] = append(channel.status2items[status], &Item{
			Identifier:  rawItem[nounsTable.IndexOfField(FieldIdentifier)].Format(""),
			Description: rawItem[nounsTable.IndexOfField(FieldDescription)].Format(""),
			Coordinal:   rawItem[nounsTable.IndexOfField(FieldCoordinal)].Format(""),
			Link:        rawItem[nounsTable.IndexOfField(FieldLink)].Format(""),
		})
	}
	for status, items := range channel.status2items {
		if status == FreshView {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			iCdn, jCdn := items[i].Coordinal, items[j].Coordinal
			// We want to items lacking a coordinal to come last
			return (iCdn < jCdn && iCdn != "") || jCdn == ""
		})
		// reset coordinals
		for i, item := range items {
			padded := strconv.Itoa(i)
			if len(padded) < 3 {
				padded = strings.Repeat("0", 3-len(padded)) + padded
			}
			item.Coordinal = padded
			nounsTable.Entries[item.Identifier][nounsTable.IndexOfField(FieldCoordinal)] = types.String(padded)
		}
	}
	return nil
}

func (mv *MainView) selectedChannel(g *gocui.Gui) (*channel, error) {
	var cy, oy int
	view, err := g.View(ResourcesView)
	if err != nil && err != gocui.ErrUnknownView {
		return nil, err
	} else if err == nil {
		_, cy = view.Cursor()
		_, oy = view.Origin()
	}
	if oy+cy >= len(mv.domains[mv.selectedDomain].channels) {
		return nil, nil
	}
	return mv.name2channel[mv.domains[mv.selectedDomain].channels[oy+cy]], nil
}
