package ui

import (
	"os/exec"
	"strings"

	"github.com/jroimartin/gocui"
)

const (
	StepView   = "steps"
	DetailView = "details"
)

type step struct {
	description string
	details     string
}

// A MainView is the overall view including a list of resources
type MainView struct {
	title    string
	steps    []step
	selected bool
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(title string, g *gocui.Gui, text string) (*MainView, error) {
	steps := parseSteps(text)
	mv := &MainView{
		title: title,
		steps: steps,
	}
	return mv, nil
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	steps, err := g.SetView(StepView, 0, 0, maxX-1, maxY-7)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	steps.Title = mv.title
	steps.Clear()
	steps.Highlight = true
	steps.SelBgColor = gocui.ColorWhite
	steps.SelFgColor = gocui.ColorBlack
	g.SetCurrentView(StepView)
	for _, step := range mv.steps {
		desc := step.description
		if len(desc) > maxX {
			desc = step.description[:maxX]
		}
		steps.Write([]byte(desc + strings.Repeat(" ", maxX-len(desc)) + "\n"))
	}
	details, err := g.SetView(DetailView, 0, maxY-7, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	details.Clear()
	err = mv.populateDetails(g)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) populateDetails(g *gocui.Gui) error {
	steps, err := g.View(StepView)
	if err != nil {
		return err
	}
	details, err := g.View(DetailView)
	if err != nil {
		return err
	}
	_, cy := steps.Cursor()
	_, oy := steps.Origin()
	ix := cy + oy
	step := mv.steps[ix]
	details.Clear()
	details.Write([]byte(step.details))
	return nil
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	err := g.SetKeybinding(StepView, 'j', gocui.ModNone, mv.incrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(StepView, 'k', gocui.ModNone, mv.decrementCursor)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(StepView, gocui.KeyEnter, gocui.ModNone, mv.selectItem)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) incrementCursor(g *gocui.Gui, v *gocui.View) error {
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	cap := len(mv.steps)
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

func (mv *MainView) selectItem(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	ix := oy + cy
	step := mv.steps[ix]
	details := strings.TrimSuffix(step.details, "\n")
	if mv.selected {
		details = "\n"
	}
	// XXX hard-coding the tmux path is not portable
	cmd := exec.Command("/usr/local/bin/tmux", "send", "-t", "bottom-right", "--", details)
	err := cmd.Run()
	if err != nil {
		return err
	}
	mv.selected = !mv.selected
	if !mv.selected && ix < len(mv.steps) - 1 {
		return mv.incrementCursor(g, v)
	}
	return nil
}

func parseSteps(text string) []step {
	items := strings.Split(text, "\n- ")
	steps := []step{}
	for _, item := range items {
		lines := strings.Split(item, "\n")
		if lines[0] == "" {
			continue
		}
		details := ""
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "```") || line == "" {
				continue
			}
			details += strings.TrimSpace(line) + "\n"
		}
		// some basic sanitizing
		steps = append(steps, step{
			description: lines[0],
			details:     details,
		})
	}
	return steps
}
