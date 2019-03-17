package ui

import (
	"io/ioutil"
	"strings"

	"github.com/jroimartin/gocui"
)

// A PromptHandler is the interface to the prompt Editor. It handles
// character inputs so it will be responsible for getting user input
// and alerting the user of errors
type PromptHandler struct {
	Callback func(contents string, finish bool, err error)
}

// Edit handles keyboard inputs when in prompt mode
func (ph *PromptHandler) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyDelete:
		v.EditDelete(false)
	case key == gocui.KeyInsert:
		v.Overwrite = !v.Overwrite
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
	case key == gocui.KeyArrowRight:
		v.MoveCursor(1, 0, false)
	case key == gocui.KeyEsc:
		// TODO handle error
		// TODO the MainView is responsible for clearing. This is a bad
		// abstraction
		v.SetCursor(0, 0)
		contents, err := ioutil.ReadAll(v)
		ph.Callback(string(contents), false, err)
	case key == gocui.KeyEnter:
		v.SetCursor(0, 0)
		contents, err := ioutil.ReadAll(v)
		ph.Callback(strings.TrimSuffix(string(contents), "\n"), true, err)
	}
}
