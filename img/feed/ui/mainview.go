package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/lib/go/timedb"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"golang.org/x/sync/errgroup"
)

const (
	blackTextEscape = "\033[30m"
	whiteBackEscape = "\033[47m"
	boldTextEscape  = "\033[1m"
	resetEscape     = "\033[0m"
)

var (
	ctx = context.Background()
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int

type MainViewDomains int

const (
	MainViewModeListBar MainViewMode = iota
	MainViewModePromptForNewEntry
	MainViewModeReturnFromPrompt
)

const (
	MainViewFixtureDomains MainViewDomains = iota
	MainViewInitiativesDomains
	MainViewWorkstreamDomains
)

// A MainView is the overall view including a resource list
// and a detailed view of the current resource
type MainView struct {
	dbms   api.JQL_DBMS
	tables map[string]*jqlpb.TableMeta

	Mode      MainViewMode
	DomainSet MainViewDomains

	ignored     map[string](map[string]bool) // stores a map from feed name to a set of ignored entries
	ignoredPath string
	returnArgs  []string

	selectedDomain int
	domains        []*domain
	id2channel     map[string]*channel

	channelToSelect string // Used to specify which channel should be initially selected when feed starts up

	newTaskDescription string
	newTaskInsertionPK string
	newTaskView        string
}

type domain struct {
	name     string
	project  bool
	channels []string
}

type channel struct {
	row          *jqlpb.Row
	status2items map[string][]*Item
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, dbms api.JQL_DBMS, ignoredPath string, returnArgs []string, selected string) (*MainView, error) {
	mv := &MainView{
		dbms: dbms,

		ignoredPath: ignoredPath,
		returnArgs:  returnArgs,
		id2channel:  map[string]*channel{},
	}
	tables, err := api.GetTables(ctx, mv.dbms)
	if err != nil {
		return nil, err
	}
	mv.tables = tables
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
	err = mv.refreshView(g)
	if err != nil {
		return nil, err
	}
	if selected != "" {
		for i, domain := range mv.renderDomains() {
			for _, channel := range domain.channels {
				if channel == selected {
					mv.selectedDomain = i
					mv.channelToSelect = channel
				}
			}
		}
	}
	return mv, mv.refreshView(g)
}

func (mv *MainView) gatherDomains() (map[string]string, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: mv.renderTable(timedb.TableAssertions),
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: timedb.FieldRelation,
						Match:  &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ".Domain"}},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	noun2domain := map[string]string{}
	for _, row := range resp.Rows {
		arg0 := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldArg0)].Formatted
		arg1 := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldArg1)].Formatted
		if !strings.HasPrefix(arg0, "nouns ") {
			continue
		}
		if !strings.HasPrefix(arg1, "@{nouns ") {
			continue
		}
		if !strings.HasSuffix(arg1, "}") {
			continue
		}
		source := arg0[len("nouns "):]
		dst := arg1[len("@{nouns ") : len(arg1)-1]
		noun2domain[source] = dst
	}
	return noun2domain, nil
}

func (mv *MainView) fetchResources() error {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: mv.renderTable(timedb.TableNouns),
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column:  timedb.FieldFeed,
						Match:   &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
						Negated: true,
					},
					{
						Column:  timedb.FieldStatus,
						Match:   &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: timedb.StatusHabitual}},
					},
				},
			},
		},
		OrderBy: timedb.FieldIdentifier,
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
	plan2description := map[string]string{}
	for _, row := range resp.Rows {
		entryName := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldIdentifier)].Formatted
		modifier := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldModifier)].Formatted
		if modifier == timedb.ValuePlanModifier {
			plan2description[entryName] = row.Entries[api.IndexOfField(resp.Columns, timedb.FieldNounDescription)].Formatted
		}
	}
	for _, row := range resp.Rows {
		if !isTrackedFeed(row, resp.Columns) {
			continue
		}
		entryName := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldIdentifier)].Formatted
		parent := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldParent)].Formatted
		domainName := noun2domain[entryName]
		if domainName == "" {
			domainName = "other"
		}
		isProject := false
		if description, ok := plan2description[entryName]; ok {
			domainName = description
			isProject = true
		}
		if description, ok := plan2description[parent]; ok {
			domainName = description
			isProject = true
		}
		if _, ok := domains[domainName]; !ok {
			domains[domainName] = &domain{name: domainName, project: isProject}
		}
		domain := domains[domainName]
		name := row.Entries[api.IndexOfField(resp.Columns, timedb.FieldIdentifier)].Formatted
		mv.id2channel[name] = &channel{
			row:          row,
			status2items: map[string][]*Item{},
		}
		domain.channels = append(domain.channels, name)
		if mv.ignored[entryName] == nil {
			mv.ignored[entryName] = map[string]bool{}
		}
	}
	mv.domains = []*domain{}
	for _, domain := range domains {
		mv.domains = append(mv.domains, domain)
	}
	sort.Slice(mv.domains, func(i, j int) bool {
		iName, jName := mv.domains[i].name, mv.domains[j].name
		// We want to show generic Attention Domains last
		return (iName < jName && iName != "other") || jName == "other"
	})

	return nil
}

func (mv *MainView) fetchNewItems(g *gocui.Gui, v *gocui.View) error {
	allContexts, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: timedb.TableContexts})
	if err != nil {
		return err
	}
	parent2context := map[string]string{}
	for _, row := range allContexts.Rows {
		parent2context[row.Entries[api.IndexOfField(allContexts.Columns, timedb.FieldParent)].Formatted] = row.Entries[api.IndexOfField(allContexts.Columns, timedb.FieldCode)].Formatted
	}

	allItems, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: mv.renderTable(timedb.TableNouns)})
	if err != nil {
		return err
	}
	allIDs := map[string]bool{}
	for _, row := range allItems.Rows {
		allIDs[row.Entries[api.IndexOfField(allItems.Columns, timedb.FieldIdentifier)].Formatted] = true
	}
	group := new(errgroup.Group)
	semaphore := make(chan bool, 5) // Limit parallel requests
	for _, domain := range mv.renderDomains() {
		for _, name := range domain.channels {
			entry := mv.id2channel[name]
			entryName := entry.row.Entries[api.IndexOfField(allItems.Columns, timedb.FieldIdentifier)].Formatted
			channel := mv.id2channel[entryName]
			channel.status2items[timedb.StatusFresh] = nil

			feedURL := entry.row.Entries[api.IndexOfField(allItems.Columns, timedb.FieldFeed)].Formatted
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
					return fmt.Errorf("Failed to fetch feed for %s: %s", entry.row.Entries[api.IndexOfField(allItems.Columns, timedb.FieldIdentifier)].Formatted, err)
				}
				for _, item := range latest {
					if !(allIDs[item.Identifier] || mv.ignored[entryName][item.Identifier]) {
						channel.status2items[timedb.StatusFresh] = append(channel.status2items[timedb.StatusFresh], item)
					}
				}
				return nil
			})
		}
	}
	return group.Wait()
}

func (mv *MainView) addFreshItem(g *gocui.Gui, v *gocui.View, status string) error {
	resources, err := g.View(timedb.ResourcesView)
	if err != nil {
		return err
	}
	_, selectedResource := resources.Cursor()
	nounsTable, ok := mv.tables[mv.renderTable(timedb.TableNouns)]
	if !ok {
		return fmt.Errorf("expected nouns table to exist")
	}
	feed := mv.id2channel[mv.renderDomains()[mv.selectedDomain].channels[selectedResource]]
	entryName := feed.row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldIdentifier)].Formatted
	_, cy := v.Cursor()
	_, oy := v.Origin()
	item := mv.id2channel[entryName].status2items[timedb.StatusFresh][oy+cy]
	fields := map[string]string{
		timedb.FieldLink:        item.Link,
		timedb.FieldParent:      entryName,
		timedb.FieldNounDescription: item.Description,
		timedb.FieldContext:     item.Context,
		timedb.FieldStatus:      status,
	}
	request := &jqlpb.WriteRowRequest{
		Table:      mv.renderTable(timedb.TableNouns),
		Pk:         item.Identifier,
		Fields:     fields,
		InsertOnly: true,
	}
	_, err = mv.dbms.WriteRow(ctx, request)
	if err != nil {
		// TODO would be good to use a specific error type
		if strings.HasPrefix(err.Error(), "Row already exists") {
			for i := 1; i < 100; i++ {
				request.Pk = fmt.Sprintf("%s (%02d)", item.Identifier, i)
				_, err = mv.dbms.WriteRow(ctx, request)
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

	channel := mv.id2channel[entryName]
	channel.status2items[timedb.StatusFresh] = append(channel.status2items[timedb.StatusFresh][:oy+cy], channel.status2items[timedb.StatusFresh][oy+cy+1:]...)
	return mv.refreshView(g)
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if v.Name() == timedb.NewTaskView {
		if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
			if len(mv.newTaskDescription) != 0 {
				mv.newTaskDescription = mv.newTaskDescription[:len(mv.newTaskDescription)-1]
			}
		} else if key == gocui.KeyEnter {
			mv.Mode = MainViewModeReturnFromPrompt
		} else if key == gocui.KeySpace {
			mv.newTaskDescription += " "
		} else {
			mv.newTaskDescription += string(ch)
		}
	}
	return
}

func (mv *MainView) layoutDomains(g *gocui.Gui, domainHeight int) error {
	maxX, _ := g.Size()
	domains, err := g.SetView(timedb.DomainView, 0, 0, maxX-1, domainHeight)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	domains.Clear()
	if len(mv.renderDomains()) == 0 {
		return nil
	}
	domainWidth := maxX / len(mv.renderDomains())
	for i, domain := range mv.renderDomains() {
		name := "  "
		if strings.HasPrefix(domain.name, "my ") {
			// HACK strip domain of unnecessary contextualizing prefix. A better approach to this
			// would be to use a "my" context code for my particular things and then show the description
			// of the domain but that's more ambitious than worthwhile for the moment
			name += domain.name[len("my "):]
		} else {
			name += domain.name
		}
		totalFresh := 0
		for _, channel := range domain.channels {
			totalFresh += len(mv.id2channel[channel].status2items[timedb.StatusFresh])
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
	switch mv.Mode {
	case MainViewModePromptForNewEntry:
		return mv.promptForNewEntry(g)
	case MainViewModeReturnFromPrompt:
		err := mv.insertNewTask(g)
		if err != nil {
			return err
		}
		err = g.DeleteView(timedb.NewTaskView)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		_, err = g.SetCurrentView(mv.newTaskView)
		if err != nil {
			return err
		}
		mv.Mode = MainViewModeListBar
		return mv.Layout(g)
	default:
		return mv.pipelinesLayout(g)
	}
}

func (mv *MainView) pipelinesLayout(g *gocui.Gui) error {
	nounsTable, ok := mv.tables[mv.renderTable(timedb.TableNouns)]
	if !ok {
		return fmt.Errorf("expected nouns table to exist -- %#v", mv.tables)
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
	active, err := g.SetView(timedb.Stage4View, resourcesWidth+1, pipeOffset(0), maxX-1, pipeOffset(0)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	planning, err := g.SetView(timedb.Stage3View, resourcesWidth+1, pipeOffset(1), maxX-1, pipeOffset(1)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	stage2, err := g.SetView(timedb.Stage2View, resourcesWidth+1, pipeOffset(2), maxX-1, pipeOffset(2)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	stage1, err := g.SetView(timedb.Stage1View, resourcesWidth+1, pipeOffset(3), maxX-1, pipeOffset(3)+pipeHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	resources, err := g.SetView(timedb.ResourcesView, 0, domainHeight+1, resourcesWidth, maxY-10)
	if err != nil && err == gocui.ErrUnknownView {
		_, err = g.SetCurrentView(timedb.ResourcesView)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	resources.Clear()
	resourcesSelected := g.CurrentView().Name() == timedb.ResourcesView
	stats, err := g.SetView(timedb.StatsView, 0, maxY-9, resourcesWidth, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	stats.Clear()
	statsContents, err := mv.statsContents(g)
	if err != nil {
		return err
	}
	stats.Write(statsContents)
	for _, view := range []*gocui.View{active, planning, stage2, stage1, resources} {
		view.SelBgColor = gocui.ColorWhite
		view.SelFgColor = gocui.ColorBlack
		view.Highlight = view == g.CurrentView()
		view.Clear()
	}

	if mv.channelToSelect != "" {
		for ix, name := range mv.renderDomains()[mv.selectedDomain].channels {
			if name == mv.channelToSelect {
				err = resources.SetCursor(0, ix)
				if err != nil {
					return err
				}
			}
		}
		mv.channelToSelect = ""
	}
	if len(mv.renderDomains()) == 0 {
		return nil
	}
	for ix, name := range mv.renderDomains()[mv.selectedDomain].channels {
		channel := mv.id2channel[name]
		description := channel.row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldNounDescription)].Formatted
		if len(channel.status2items[timedb.StatusFresh]) > 0 {
			description += fmt.Sprintf(" %s(%d)%s", boldTextEscape, len(channel.status2items[timedb.StatusFresh]), resetEscape)
		}
		// If resources are not selected we'll bold the selected channel
		_, cy := resources.Cursor()
		_, oy := resources.Origin()
		if !resourcesSelected && ix == (cy+oy) {
			fmt.Fprintf(resources, "  %s%s%s\n", boldTextEscape, description, resetEscape)
		} else {
			fmt.Fprintf(resources, "  %s\n", description)
		}
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}

	for status, items := range channel.status2items {
		for _, item := range items {
			stages := mv.status2stage(channel)
			stage, ok := stages[status]
			if !ok {
				continue
			}
			view, err := g.View(stage)
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
	return nil
}

func (mv *MainView) promptForNewEntry(g *gocui.Gui) error {
	maxX, _ := g.Size()
	newTaskView, err := g.SetView(timedb.NewTaskView, 4, 5, maxX-4, 9)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	newTaskView.Editable = true
	newTaskView.Editor = mv
	g.SetCurrentView(timedb.NewTaskView)
	newTaskView.Clear()
	newTaskView.Write([]byte("New Task Description\n"))
	newTaskView.Write([]byte(mv.newTaskDescription))
	return nil
}

func reverseMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[v] = k
	}
	return out
}

func (mv *MainView) status2stage(ch *channel) map[string]string {
	if len(ch.status2items[timedb.StatusFresh]) > 0 {
		return map[string]string{
			timedb.StatusFresh:     timedb.Stage1View,
			timedb.StatusIdea:      timedb.Stage2View,
			timedb.StatusExploring: timedb.Stage3View,
			timedb.StatusPlanning:  timedb.Stage4View,
		}
	}
	return map[string]string{
		timedb.StatusIdea:         timedb.Stage1View,
		timedb.StatusExploring:    timedb.Stage2View,
		timedb.StatusPlanning:     timedb.Stage3View,
		timedb.StatusImplementing: timedb.Stage4View,
	}
}

func (mv *MainView) SaveIgnores() error {
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
		timedb.ResourcesView: timedb.Stage1View,
		timedb.Stage1View:    timedb.Stage2View,
		timedb.Stage2View:    timedb.Stage3View,
		timedb.Stage3View:    timedb.Stage4View,
		timedb.Stage4View:    timedb.ResourcesView,
	}
	for current, next := range nextMap {
		err := g.SetKeybinding(current, 'f', gocui.ModNone, mv.fetchNewItems)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'p', gocui.ModNone, mv.switchDomainSet)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'n', gocui.ModNone, mv.switcherTo(next))
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
		err = g.SetKeybinding(current, 'l', gocui.ModNone, mv.incrementDomain)
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'h', gocui.ModNone, mv.decrementDomain)
		if err != nil {
			return err
		}
		if current == timedb.ResourcesView {
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
		err = g.SetKeybinding(current, 'o', gocui.ModNone, mv.statusSetter(timedb.StatusImplementing))
		if err != nil {
			return err
		}
		err = g.SetKeybinding(current, 'O', gocui.ModNone, mv.statusSetter(timedb.StatusIdea))
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
		if err := g.SetKeybinding(current, gocui.KeyEnter, gocui.ModNone, mv.setPromptForNewEntry); err != nil {
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
	if name == timedb.ResourcesView || name == timedb.StatusFresh {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	stage2status := reverseMap(mv.status2stage(channel))
	pk := channel.status2items[stage2status[name]][oy+cy].Identifier
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  mv.renderTable(timedb.TableNouns),
		Pk:     pk,
		Column: timedb.FieldCoordinal,
		Amount: 1,
	})
	if err != nil {
		return err
	}
	if oy+cy+1 < len(channel.status2items[stage2status[name]]) {
		successor := channel.status2items[stage2status[name]][oy+cy+1].Identifier
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  mv.renderTable(timedb.TableNouns),
			Pk:     successor,
			Column: timedb.FieldCoordinal,
			Amount: -1,
		})
		if err != nil {
			return err
		}
	}
	return mv.cursorDown(g, v)
}

func (mv *MainView) moveUpInPipe(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == timedb.ResourcesView || name == timedb.StatusFresh {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	stage2status := reverseMap(mv.status2stage(channel))
	pk := channel.status2items[stage2status[name]][oy+cy].Identifier
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  mv.renderTable(timedb.TableNouns),
		Pk:     pk,
		Column: timedb.FieldCoordinal,
		Amount: -1,
	})
	if err != nil {
		return err
	}
	if oy+cy-1 >= 0 {
		predecessor := channel.status2items[stage2status[name]][oy+cy-1].Identifier
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  mv.renderTable(timedb.TableNouns),
			Pk:     predecessor,
			Column: timedb.FieldCoordinal,
			Amount: 1,
		})
		if err != nil {
			return err
		}
	}
	return mv.cursorUp(g, v)
}

func (mv *MainView) moveDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == timedb.ResourcesView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	nounsTable, ok := mv.tables[mv.renderTable(timedb.TableNouns)]
	if !ok {
		return fmt.Errorf("Expected to find nouns table")
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	stage2status := reverseMap(mv.status2stage(channel))
	pk := channel.status2items[stage2status[name]][oy+cy].Identifier
	if stage2status[name] == timedb.StatusFresh {
		resources, err := g.View(timedb.ResourcesView)
		if err != nil {
			return err
		}
		_, roy := resources.Origin()
		_, rcy := resources.Cursor()
		row := mv.id2channel[mv.renderDomains()[mv.selectedDomain].channels[roy+rcy]].row
		entryName := row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldIdentifier)].Formatted
		mv.ignored[entryName][pk] = true
		channel := mv.id2channel[entryName]
		channel.status2items[timedb.StatusFresh] = append(channel.status2items[timedb.StatusFresh][:oy+cy], channel.status2items[timedb.StatusFresh][oy+cy+1:]...)
	} else {
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  mv.renderTable(timedb.TableNouns),
			Pk:     pk,
			Column: timedb.FieldStatus,
			Amount: -1,
		})
		if err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) incrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + 1) % len(mv.renderDomains())
	return mv.resetView(g)
}

func (mv *MainView) decrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + len(mv.renderDomains()) - 1) % len(mv.renderDomains())
	return mv.resetView(g)
}

func (mv *MainView) statusSetter(status string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return mv.setStatus(g, v, status)
	}
}

func (mv *MainView) setStatus(g *gocui.Gui, v *gocui.View, status string) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == timedb.ResourcesView {
		return nil
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	stage2status := reverseMap(mv.status2stage(channel))
	if stage2status[name] == timedb.StatusFresh {
		return mv.addFreshItem(g, v, status)
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	pk := channel.status2items[stage2status[name]][oy+cy].Identifier
	_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
		Table: mv.renderTable(timedb.TableNouns),
		Pk:    pk,
		Fields: map[string]string{
			timedb.FieldStatus: status,
		},
		UpdateOnly: true,
	})
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) moveUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	name := v.Name()
	if name == timedb.ResourcesView {
		return nil
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	stage2status := reverseMap(mv.status2stage(channel))
	status := stage2status[name]
	if status == timedb.StatusFresh {
		return mv.addFreshItem(g, v, timedb.StatusIdea)
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	pk := channel.status2items[stage2status[name]][oy+cy].Identifier
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  mv.renderTable(timedb.TableNouns),
		Pk:     pk,
		Column: timedb.FieldStatus,
		Amount: 1,
	})
	if err != nil {
		return err
	}
	if status == timedb.StatusIdea {
		arg0 := api.ConstructPolyForeign(mv.renderTable(timedb.TableNouns), pk)
		arg1 := time.Now().Format("2006-01-02")
		order := "0000"
		_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
			Table: mv.renderTable(timedb.TableAssertions),
			Pk:    fmt.Sprintf("(%q,%q,%q)", arg0, arg1, order),
			Fields: map[string]string{
				timedb.FieldRelation: ".StartDate",
				timedb.FieldArg0:     arg0,
				timedb.FieldArg1:     arg1,
				timedb.FieldOrder:    order,
			},
			InsertOnly: true,
		})
		if err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	max := 0
	if v.Name() == timedb.ResourcesView {
		max = len(mv.renderDomains()[mv.selectedDomain].channels)
	} else {
		channel, err := mv.selectedChannel(g)
		if err != nil {
			return err
		}
		stage2status := reverseMap(mv.status2stage(channel))
		max = len(channel.status2items[stage2status[v.Name()]])
	}
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	if cy+oy+1 >= max {
		return nil
	}
	if err := v.SetCursor(cx, cy+1); err != nil {
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

func (mv *MainView) GetSelectedPK(g *gocui.Gui, v *gocui.View) (string, error) {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	if v.Name() == timedb.ResourcesView {
		nounsTable, ok := mv.tables[mv.renderTable(timedb.TableNouns)]
		if !ok {
			return "", fmt.Errorf("expected nouns table to exist")
		}
		if mv.DomainSet == MainViewInitiativesDomains {
			return mv.id2channel[mv.renderDomains()[mv.selectedDomain].channels[oy+cy]].row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldNounDescription)].Formatted, nil
		}
		return mv.id2channel[mv.renderDomains()[mv.selectedDomain].channels[oy+cy]].row.Entries[api.IndexOfField(nounsTable.Columns, timedb.FieldIdentifier)].Formatted, nil
	} else {
		channel, err := mv.selectedChannel(g)
		if err != nil {
			return "", err
		}
		stage2status := reverseMap(mv.status2stage(channel))
		items := channel.status2items[stage2status[v.Name()]]
		if oy+cy >= len(items) {
			return "", nil
		}
		return items[oy+cy].Identifier, nil
	}
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	// We refresh all channels here so we can be sure the status UI reflects the current
	// state even though most likely only the current selected channel changed
	for _, chn := range mv.id2channel {
		for _, status := range []string{timedb.StatusImplementing, timedb.StatusExploring, timedb.StatusIdea, timedb.StatusPlanning} {
			chn.status2items[status] = nil
		}
	}
	rawItems, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: mv.renderTable(timedb.TableNouns)})
	if err != nil {
		return err
	}

	mv.tables[mv.renderTable(timedb.TableNouns)] = &jqlpb.TableMeta{
		Name:    rawItems.Table,
		Columns: rawItems.Columns,
	}

	for _, rawItem := range rawItems.Rows {
		channel, ok := mv.id2channel[rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldParent)].Formatted]
		if !ok {
			continue
		}
		status := rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldStatus)].Formatted
		channel.status2items[status] = append(channel.status2items[status], &Item{
			Identifier:  rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldIdentifier)].Formatted,
			Description: rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldNounDescription)].Formatted,
			Coordinal:   rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldCoordinal)].Formatted,
			Link:        rawItem.Entries[api.IndexOfField(rawItems.Columns, timedb.FieldLink)].Formatted,
		})
	}
	for _, channel := range mv.id2channel {
		for status, items := range channel.status2items {
			if status != timedb.StatusIdea && status != timedb.StatusExploring && status != timedb.StatusImplementing && status != timedb.StatusPlanning {
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
				if item.Coordinal == padded {
					continue
				}
				item.Coordinal = padded
				_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
					UpdateOnly: true,
					Table:      mv.renderTable(timedb.TableNouns),
					Pk:         item.Identifier,
					Fields: map[string]string{
						timedb.FieldCoordinal: padded,
					},
				})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (mv *MainView) selectedChannel(g *gocui.Gui) (*channel, error) {
	var cy, oy int
	view, err := g.View(timedb.ResourcesView)
	if err != nil && err != gocui.ErrUnknownView {
		return nil, err
	} else if err == nil {
		_, cy = view.Cursor()
		_, oy = view.Origin()
	}
	if len(mv.renderDomains()) == 0 {
		return nil, nil
	}
	if oy+cy >= len(mv.renderDomains()[mv.selectedDomain].channels) {
		return nil, nil
	}
	return mv.id2channel[mv.renderDomains()[mv.selectedDomain].channels[oy+cy]], nil
}

func (mv *MainView) statsContents(g *gocui.Gui) ([]byte, error) {
	colW := 5
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, nil
	}
	domainCounts := mv.domainCounts()
	allCounts := mv.allCounts()
	totalSum := 0
	relevantStatuses := []string{
		timedb.StatusFresh,
		timedb.StatusIdea,
		timedb.StatusExploring,
		timedb.StatusPlanning,
		timedb.StatusImplementing,
	}
	for _, status := range relevantStatuses {
		totalSum += allCounts[status]
	}
	stats := fmt.Sprintf(`      U    I    E    P    M

C     %s%s%s%s%s

D     %s%s%s%s%s

A     %s%s%s%s%s%s
`,
		// Channel
		fmt.Sprintf("%-*d", colW, len(channel.status2items[timedb.StatusFresh])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[timedb.StatusIdea])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[timedb.StatusExploring])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[timedb.StatusPlanning])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[timedb.StatusImplementing])),

		// Domain
		fmt.Sprintf("%-*d", colW, domainCounts[timedb.StatusFresh]),
		fmt.Sprintf("%-*d", colW, domainCounts[timedb.StatusIdea]),
		fmt.Sprintf("%-*d", colW, domainCounts[timedb.StatusExploring]),
		fmt.Sprintf("%-*d", colW, domainCounts[timedb.StatusPlanning]),
		fmt.Sprintf("%-*d", colW, domainCounts[timedb.StatusImplementing]),

		// All
		fmt.Sprintf("%-*d", colW, allCounts[timedb.StatusFresh]),
		fmt.Sprintf("%-*d", colW, allCounts[timedb.StatusIdea]),
		fmt.Sprintf("%-*d", colW, allCounts[timedb.StatusExploring]),
		fmt.Sprintf("%-*d", colW, allCounts[timedb.StatusPlanning]),
		fmt.Sprintf("%-*d", colW, allCounts[timedb.StatusImplementing]),
		fmt.Sprintf("%-*d", colW, totalSum),
	)
	return []byte(stats), nil
}

func (mv *MainView) domainCounts() map[string]int {
	counts := map[string]int{}
	if len(mv.renderDomains()) == 0 {
		return counts
	}
	domain := mv.renderDomains()[mv.selectedDomain]
	for _, id := range domain.channels {
		for status, items := range mv.id2channel[id].status2items {
			counts[status] += len(items)
		}
	}
	return counts
}

func (mv *MainView) setPromptForNewEntry(g *gocui.Gui, v *gocui.View) error {
	if v.Name() == timedb.ResourcesView {
		return nil
	}
	selectedPk, err := mv.GetSelectedPK(g, v)
	if err != nil {
		return err
	}
	mv.newTaskInsertionPK = selectedPk
	mv.newTaskDescription = ""
	mv.newTaskView = v.Name()
	mv.Mode = MainViewModePromptForNewEntry
	return nil
}

func (mv *MainView) insertNewTask(g *gocui.Gui) error {
	// TODO in addition to duplicating existing task fields (to get an appropriate coordinal) it'd be good to
	/// run the pk setter on the new item so context is guaranteed to be correct
	toDupe, err := mv.dbms.GetRow(ctx, &jqlpb.GetRowRequest{
		Table: mv.renderTable(timedb.TableNouns),
		Pk:    mv.newTaskInsertionPK,
	})
	if err != nil && !api.IsNotExistError(err) {
		return err
	}
	fields := map[string]string{
		timedb.FieldNounDescription: mv.newTaskDescription,
	}
	pk := mv.newTaskDescription
	if toDupe != nil {
		entryCtx := toDupe.Row.Entries[api.IndexOfField(toDupe.Columns, timedb.FieldContext)].Formatted
		if entryCtx != "" {
			pk = fmt.Sprintf("[%s] %s", entryCtx, pk)
		}
		fields[timedb.FieldContext] = entryCtx
		fields[timedb.FieldCoordinal] = toDupe.Row.Entries[api.IndexOfField(toDupe.Columns, timedb.FieldCoordinal)].Formatted
		fields[timedb.FieldModifier] = toDupe.Row.Entries[api.IndexOfField(toDupe.Columns, timedb.FieldModifier)].Formatted
	}
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	fields[timedb.FieldParent] = channel.row.Entries[api.GetPrimary(mv.tables[mv.renderTable(timedb.TableNouns)].Columns)].Formatted
	stage2status := reverseMap(mv.status2stage(channel))
	fields[timedb.FieldStatus] = stage2status[mv.newTaskView]
	request := &jqlpb.WriteRowRequest{
		Table:      mv.renderTable(timedb.TableNouns),
		Pk:         pk,
		Fields:     fields,
		InsertOnly: true,
	}
	_, err = mv.dbms.WriteRow(ctx, request)
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) allCounts() map[string]int {
	counts := map[string]int{}
	for _, d := range mv.renderDomains() {
		for _, chName := range d.channels {
			if channel, ok := mv.id2channel[chName]; ok {
				for status, items := range channel.status2items {
					counts[status] += len(items)
				}
			}
		}
	}
	return counts
}

// resetView resets all cursors and the selected view for use
// when user switches the selected domain
func (mv *MainView) resetView(g *gocui.Gui) error {
	for _, viewName := range []string{timedb.ResourcesView, timedb.Stage1View, timedb.Stage2View, timedb.Stage3View, timedb.Stage4View} {
		view, err := g.View(viewName)
		if err != nil {
			return err
		}
		if err := view.SetCursor(0, 0); err != nil {
			return err
		}
		if err := view.SetOrigin(0, 0); err != nil {
			return err
		}
	}
	if _, err := g.SetCurrentView(timedb.ResourcesView); err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) switchDomainSet(g *gocui.Gui, v *gocui.View) error {
	switch mv.DomainSet {
	case MainViewFixtureDomains:
		mv.DomainSet = MainViewInitiativesDomains
	case MainViewInitiativesDomains:
		mv.DomainSet = MainViewWorkstreamDomains
	case MainViewWorkstreamDomains:
		mv.DomainSet = MainViewFixtureDomains
	default:
		mv.DomainSet = MainViewInitiativesDomains
	}
	if mv.DomainSet != MainViewWorkstreamDomains {
		// when switching from initiatives to workstreams, keep the selected project
		mv.selectedDomain = 0
	}
	err := mv.fetchResources()
	if err != nil {
		return err
	}
	return mv.resetView(g)
}

func (mv *MainView) renderTable(base string) string {
	prefix := ""
	switch mv.DomainSet {
	case MainViewInitiativesDomains:
		prefix = "vt.project_initiative_"
	}
	return prefix + base
}

func (mv *MainView) renderDomains() []*domain {
	var rendered []*domain
	for _, d := range mv.domains {
		if mv.DomainSet == MainViewWorkstreamDomains != !d.project {
			rendered = append(rendered, d)
		}
	}
	return rendered
}

func isAutomatedFeed(row *jqlpb.Row, columns []*jqlpb.Column) bool {
	feed := row.Entries[api.IndexOfField(columns, timedb.FieldFeed)].Formatted
	return feed != timedb.FeedManual
}

func isTrackedFeed(row *jqlpb.Row, columns []*jqlpb.Column) bool {
	feed := row.Entries[api.IndexOfField(columns, timedb.FieldFeed)].Formatted
	return strings.Contains(feed, "://") || feed == timedb.FeedManual
}
