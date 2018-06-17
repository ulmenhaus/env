package main

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/netlog/parse"
)

type SelectableList struct {
	items    []string // should be String() interface
	selected int
}

func (sl *SelectableList) Down(g *gocui.Gui, v *gocui.View) error {
	if sl.selected < len(sl.items)-1 {
		sl.selected++
	}
	return nil
}

func (sl *SelectableList) Up(g *gocui.Gui, v *gocui.View) error {
	if sl.selected > 0 {
		sl.selected--
	}
	return nil
}

func (sl *SelectableList) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView("list", 0, 0, maxX-1, maxY-1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	content := ""
	for i, item := range sl.items {
		if i == sl.selected {
			content += ">>> "
		} else {
			content += "    "
		}
		content += item + "\n"
	}
	v.Clear()
	fmt.Fprintf(v, content)
	return nil
}

func (s1 *SelectableList) Add(item string) {
	// TODO thread safe
	s1.items = append(s1.items, item)
}

func main() {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		panic(err)
	}
	sl := &SelectableList{
		items: []string{},
	}

	defer g.Close()

	g.SetManagerFunc(sl.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, sl.Down); err != nil {
		panic(err)
	}

	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, sl.Up); err != nil {
		panic(err)
	}

	// TODO pass args from cli
	go func() {
		// XXX For PoC only do IPv4 traffic
		cmd := exec.Command("tcpdump", "-i", "any", "-vv", "-XX", "ip")
		p, err := parse.NewTCPDumpParser()
		if err != nil {
			panic(err)
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		// TODO collect stderr
		err = cmd.Start()
		if err != nil {
			panic(err)
		}
		eventChan, errChan := p.ParseStream(out)
		for event := range eventChan {
			g.Update(func(g *gocui.Gui) error {
				sl.Add(fmt.Sprintf("frame of size: %d", len(event.Frame)))
				return nil
			})
		}
		err = <-errChan
		if err != nil && err != io.EOF {
			panic(err)
		}
	}()
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		panic(err)
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
