package main

import "github.com/egoist/mygo"

// Window close requests hide Tree without destroying its window, timers or
// global shortcut. A deliberate application quit still closes the window.
func (a *app) closeWindow(e *mygo.CloseEvent) {
	if a.quitting {
		return
	}
	e.PreventDefault()
	if !a.saveBeforeBackground() {
		return
	}
	a.hide()
}

func (a *app) beforeQuit(e *mygo.QuitEvent) {
	a.quitting = false
	if !a.saveBeforeBackground() {
		e.PreventDefault()
		return
	}
	a.quitting = true
}

func (a *app) saveBeforeBackground() bool {
	if err := a.flush(); err != nil {
		a.report(err)
		if a.win != nil {
			a.win.Update(func() {})
		}
		return false
	}
	return true
}
