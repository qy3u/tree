package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
	"os"
	"path/filepath"
)

func main() {
	mygo.App.SetName("Tree")
	dir := os.Getenv("TREE_NOTES_DATA_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(base, "Tree Notes")
	}
	s, err := openStore(dir)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	a, err := newApp(s)
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Label: "文件", Submenu: a.fileMenu()}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleFormatMenu}, {Role: mygo.RoleWindowMenu}}))
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Tree — 本地笔记", TitleBarStyle: mygo.TitleBarHidden, Width: 1120, Height: 800, MinWidth: 850, MinHeight: 580, StateKey: "notes", Content: ui.View(a.view)})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if err := a.flush(); err != nil {
				a.report(err)
				e.PreventDefault()
				a.win.Update(func() {})
			}
		})
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			if err := a.flush(); err != nil {
				a.report(err)
				e.PreventDefault()
				a.win.Update(func() {})
			}
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) fileMenu() []*mygo.MenuItem {
	action := func(label, key string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: key, Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
			if a.win != nil {
				a.win.Update(fn)
			}
		}}
	}
	return []*mygo.MenuItem{
		action("新建页面", "CmdOrCtrl+N", func() { a.addPage("") }),
		action("新建子页面", "Shift+CmdOrCtrl+N", func() { a.addPage(a.selected) }),
		action("新建标签页", "CmdOrCtrl+T", func() { a.showTabPicker() }),
		mygo.Separator(),
		action("关闭标签页", "CmdOrCtrl+W", func() { a.closeTab(a.activeTab) }),
		{Role: mygo.RoleClose, Label: "关闭窗口", Accelerator: "Shift+CmdOrCtrl+W"},
	}
}
