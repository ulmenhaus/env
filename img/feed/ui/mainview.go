package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/api"
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

const (
	MainViewModeListBar MainViewMode = iota
)

// A MainView is the overall view including a resource list
// and a detailed view of the current resource
type MainView struct {
	dbms   api.JQL_DBMS
	tables map[string]*jqlpb.TableMeta

	Mode MainViewMode

	ignored     map[string](map[string]bool) // stores a map from feed name to a set of ignored entries
	ignoredPath string
	returnArgs  []string

	selectedDomain int
	domains        []*domain
	id2channel     map[string]*channel
}

type domain struct {
	name     string
	channels []string
}

type channel struct {
	row          *jqlpb.Row
	status2items map[string][]*Item
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(g *gocui.Gui, dbms api.JQL_DBMS, ignoredPath string, returnArgs []string) (*MainView, error) {
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
	return mv, mv.refreshView(g)
}

func (mv *MainView) gatherDomains() (map[string]string, error) {
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableAssertions,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column: FieldRelation,
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
		arg0 := row.Entries[api.IndexOfField(resp.Columns, FieldArg0)].Formatted
		arg1 := row.Entries[api.IndexOfField(resp.Columns, FieldArg1)].Formatted
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
	resp, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{
		Table: TableNouns,
		Conditions: []*jqlpb.Condition{
			{
				Requires: []*jqlpb.Filter{
					{
						Column:  FieldFeed,
						Match:   &jqlpb.Filter_EqualMatch{&jqlpb.EqualMatch{Value: ""}},
						Negated: true,
					},
				},
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
	for _, row := range resp.Rows {
		feed := row.Entries[api.IndexOfField(resp.Columns, FieldFeed)].Formatted
		if (!strings.Contains(feed, "://")) && feed != "manual" {
			continue
		}
		entryName := row.Entries[api.IndexOfField(resp.Columns, FieldIdentifier)].Formatted
		domainName := noun2domain[entryName]
		if domainName == "" {
			domainName = "other"
		}
		if _, ok := domains[domainName]; !ok {
			domains[domainName] = &domain{name: domainName}
		}
		domain := domains[domainName]
		name := row.Entries[api.IndexOfField(resp.Columns, FieldIdentifier)].Formatted
		mv.id2channel[name] = &channel{
			row:          row,
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
		return (iName < jName && iName != "other") || jName == "other"
	})

	return nil
}

func (mv *MainView) fetchNewItems(g *gocui.Gui, v *gocui.View) error {
	allContexts, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: TableContexts})
	if err != nil {
		return err
	}
	parent2context := map[string]string{}
	for _, row := range allContexts.Rows {
		parent2context[row.Entries[api.IndexOfField(allContexts.Columns, FieldParent)].Formatted] = row.Entries[api.IndexOfField(allContexts.Columns, FieldCode)].Formatted
	}

	allItems, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: TableNouns})
	if err != nil {
		return err
	}
	allIDs := map[string]bool{}
	for _, row := range allItems.Rows {
		allIDs[row.Entries[api.IndexOfField(allItems.Columns, FieldIdentifier)].Formatted] = true
	}
	group := new(errgroup.Group)
	semaphore := make(chan bool, 5) // Limit parallel requests
	for _, domain := range mv.domains {
		for _, name := range domain.channels {
			entry := mv.id2channel[name]
			entryName := entry.row.Entries[api.IndexOfField(allItems.Columns, FieldIdentifier)].Formatted
			channel := mv.id2channel[entryName]
			channel.status2items[FreshView] = nil

			feedURL := entry.row.Entries[api.IndexOfField(allItems.Columns, FieldFeed)].Formatted
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
					return fmt.Errorf("Failed to fetch feed for %s: %s", entry.row.Entries[api.IndexOfField(allItems.Columns, FieldIdentifier)].Formatted, err)
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
	nounsTable, ok := mv.tables[TableNouns]
	if !ok {
		return fmt.Errorf("expected resources table to exist")
	}
	feed := mv.id2channel[mv.domains[mv.selectedDomain].channels[selectedResource]]
	entryName := feed.row.Entries[api.IndexOfField(nounsTable.Columns, FieldIdentifier)].Formatted
	_, cy := v.Cursor()
	_, oy := v.Origin()
	item := mv.id2channel[entryName].status2items[FreshView][oy+cy]
	fields := map[string]string{
		FieldLink:        item.Link,
		FieldParent:      entryName,
		FieldDescription: item.Description,
		FieldContext:     item.Context,
	}
	request := &jqlpb.WriteRowRequest{
		Table:      TableNouns,
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
			totalFresh += len(mv.id2channel[channel].status2items[FreshView])
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
	nounsTable, ok := mv.tables[TableNouns]
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
	resources, err := g.SetView(ResourcesView, 0, domainHeight+1, resourcesWidth, maxY-10)
	if err != nil && err == gocui.ErrUnknownView {
		_, err = g.SetCurrentView(ResourcesView)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	resources.Clear()
	resourcesSelected := g.CurrentView().Name() == ResourcesView
	stats, err := g.SetView(StatsView, 0, maxY-9, resourcesWidth, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	stats.Clear()
	statsContents, err := mv.statsContents(g)
	if err != nil {
		return err
	}
	stats.Write(statsContents)
	for _, view := range []*gocui.View{active, pending, idea, fresh, resources} {
		view.SelBgColor = gocui.ColorWhite
		view.SelFgColor = gocui.ColorBlack
		view.Highlight = view == g.CurrentView()
		view.Clear()
	}

	for ix, name := range mv.domains[mv.selectedDomain].channels {
		channel := mv.id2channel[name]
		description := channel.row.Entries[api.IndexOfField(nounsTable.Columns, FieldDescription)].Formatted
		if len(channel.status2items[FreshView]) > 0 {
			description += fmt.Sprintf(" %s(%d)%s", boldTextEscape, len(channel.status2items[FreshView]), resetEscape)
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
	_, err := mv.dbms.Persist(ctx, &jqlpb.PersistRequest{})
	return err
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
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	pk := channel.status2items[name][oy+cy].Identifier
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  TableNouns,
		Pk:     pk,
		Column: FieldCoordinal,
		Amount: 1,
	})
	if err != nil {
		return err
	}
	if oy+cy+1 < len(channel.status2items[name]) {
		successor := channel.status2items[name][oy+cy+1].Identifier
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  TableNouns,
			Pk:     successor,
			Column: FieldCoordinal,
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
	if name == ResourcesView || name == FreshView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return err
	}
	pk := channel.status2items[name][oy+cy].Identifier
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  TableNouns,
		Pk:     pk,
		Column: FieldCoordinal,
		Amount: -1,
	})
	if err != nil {
		return err
	}
	if oy+cy-1 >= 0 {
		predecessor := channel.status2items[name][oy+cy-1].Identifier
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  TableNouns,
			Pk:     predecessor,
			Column: FieldCoordinal,
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
	if name == ResourcesView {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	nounsTable, ok := mv.tables[TableNouns]
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
		row := mv.id2channel[mv.domains[mv.selectedDomain].channels[roy+rcy]].row
		entryName := row.Entries[api.IndexOfField(nounsTable.Columns, FieldIdentifier)].Formatted
		mv.ignored[entryName][pk] = true
		channel := mv.id2channel[entryName]
		channel.status2items[FreshView] = append(channel.status2items[FreshView][:oy+cy], channel.status2items[FreshView][oy+cy+1:]...)
	} else {
		_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
			Table:  TableNouns,
			Pk:     pk,
			Column: FieldStatus,
			Amount: -1,
		})
		if err != nil {
			return err
		}
	}
	return mv.refreshView(g)
}

func (mv *MainView) incrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + 1) % len(mv.domains)
	return mv.resetView(g)
}

func (mv *MainView) decrementDomain(g *gocui.Gui, v *gocui.View) error {
	mv.selectedDomain = (mv.selectedDomain + len(mv.domains) - 1) % len(mv.domains)
	return mv.resetView(g)
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
	_, err = mv.dbms.IncrementEntry(ctx, &jqlpb.IncrementEntryRequest{
		Table:  TableNouns,
		Pk:     pk,
		Column: FieldStatus,
		Amount: 1,
	})
	if err != nil {
		return err
	}
	return mv.refreshView(g)
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	max := 0
	if v.Name() == ResourcesView {
		max = len(mv.domains[mv.selectedDomain].channels)
	} else {
		channel, err := mv.selectedChannel(g)
		if err != nil {
			return err
		}
		max = len(channel.status2items[v.Name()])
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

func (mv *MainView) goToJQL(extraArgs ...string) error {
	err := mv.save()
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(JQLName)
	if err != nil {
		return err
	}

	args := append([]string{JQLName}, mv.returnArgs...)
	args = append(args, extraArgs...)

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (mv *MainView) GetSelectedPK(g *gocui.Gui, v *gocui.View) (string, error) {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	if v.Name() == ResourcesView {
		nounsTable, ok := mv.tables[TableNouns]
		if !ok {
			return "", fmt.Errorf("expected nouns table to exist")
		}
		return mv.id2channel[mv.domains[mv.selectedDomain].channels[oy+cy]].row.Entries[api.IndexOfField(nounsTable.Columns, FieldIdentifier)].Formatted, nil
	} else {
		channel, err := mv.selectedChannel(g)
		if err != nil {
			return "", err
		}
		return channel.status2items[v.Name()][oy+cy].Identifier, nil
	}
}

func (mv *MainView) refreshView(g *gocui.Gui) error {
	// We refresh all channels here so we can be sure the status UI reflects the current
	// state even though most likely only the current selected channel changed
	for _, chn := range mv.id2channel {
		for _, status := range []string{StatusActive, StatusPending, StatusIdea} {
			chn.status2items[status] = nil
		}
	}
	rawItems, err := mv.dbms.ListRows(ctx, &jqlpb.ListRowsRequest{Table: TableNouns})
	if err != nil {
		return err
	}

	for _, rawItem := range rawItems.Rows {
		channel, ok := mv.id2channel[rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldParent)].Formatted]
		if !ok {
			continue
		}
		status := rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldStatus)].Formatted
		channel.status2items[status] = append(channel.status2items[status], &Item{
			Identifier:  rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldIdentifier)].Formatted,
			Description: rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldDescription)].Formatted,
			Coordinal:   rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldCoordinal)].Formatted,
			Link:        rawItem.Entries[api.IndexOfField(rawItems.Columns, FieldLink)].Formatted,
		})
	}
	for _, channel := range mv.id2channel {
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
				_, err = mv.dbms.WriteRow(ctx, &jqlpb.WriteRowRequest{
					UpdateOnly: true,
					Table:      TableNouns,
					Pk:         item.Identifier,
					Fields: map[string]string{
						FieldCoordinal: padded,
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
	return mv.id2channel[mv.domains[mv.selectedDomain].channels[oy+cy]], nil
}

func (mv *MainView) statsContents(g *gocui.Gui) ([]byte, error) {
	colW := 5
	channel, err := mv.selectedChannel(g)
	if err != nil {
		return nil, err
	}
	domainCounts := mv.domainCounts()
	allCounts := mv.allCounts()
	stats := fmt.Sprintf(`      U    I    P    A

C     %s%s%s%s

D     %s%s%s%s

A     %s%s%s%s
`,
		// Channel
		fmt.Sprintf("%-*d", colW, len(channel.status2items[FreshView])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[StatusIdea])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[StatusPending])),
		fmt.Sprintf("%-*d", colW, len(channel.status2items[StatusActive])),

		// Domain
		fmt.Sprintf("%-*d", colW, domainCounts[FreshView]),
		fmt.Sprintf("%-*d", colW, domainCounts[StatusIdea]),
		fmt.Sprintf("%-*d", colW, domainCounts[StatusPending]),
		fmt.Sprintf("%-*d", colW, domainCounts[StatusActive]),

		// All
		fmt.Sprintf("%-*d", colW, allCounts[FreshView]),
		fmt.Sprintf("%-*d", colW, allCounts[StatusIdea]),
		fmt.Sprintf("%-*d", colW, allCounts[StatusPending]),
		fmt.Sprintf("%-*d", colW, allCounts[StatusActive]),
	)
	return []byte(stats), nil
}

func (mv *MainView) domainCounts() map[string]int {
	domain := mv.domains[mv.selectedDomain]
	counts := map[string]int{}
	for _, id := range domain.channels {
		for status, items := range mv.id2channel[id].status2items {
			counts[status] += len(items)
		}
	}
	return counts
}

func (mv *MainView) allCounts() map[string]int {
	counts := map[string]int{}
	for _, channel := range mv.id2channel {
		for status, items := range channel.status2items {
			counts[status] += len(items)
		}
	}
	return counts
}

// resetView resets all cursors and the selected view for use
// when user switches the selected domain
func (mv *MainView) resetView(g *gocui.Gui) error {
	for _, viewName := range []string{ResourcesView, FreshView, StatusIdea, StatusPending, StatusActive} {
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
	if _, err := g.SetCurrentView(ResourcesView); err != nil {
		return err
	}
	return mv.refreshView(g)
}
