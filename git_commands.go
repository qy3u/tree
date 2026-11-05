package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// Headless entry points intentionally stop before newApp: opening an editor
// records visits and repairs UI state, which must not contaminate an export.
func runDataCommand(args []string, defaultDir string, out io.Writer) (string, bool, error) {
	flags := flag.NewFlagSet("Tree", flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", defaultDir, "笔记库目录")
	export := flags.String("export-git", "", "导出到专用文本快照目录")
	restore := flags.String("restore-git", "", "从文本快照还原到 --data-dir 指定的空目录")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return *dir, true, nil
		}
		return *dir, true, err
	}
	if flags.NArg() != 0 {
		return *dir, true, errors.New("不支持的位置参数")
	}
	if *export != "" && *restore != "" {
		return *dir, true, errors.New("导出与还原不能同时执行")
	}
	if *restore != "" {
		explicit := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "data-dir" {
				explicit = true
			}
		})
		if !explicit {
			return *dir, true, errors.New("还原必须用 --data-dir 指定新的空目录")
		}
		err := RestoreGit(*restore, *dir)
		if err == nil {
			fmt.Fprintln(out, "已还原笔记库：", *dir)
		}
		return *dir, true, err
	}
	if *export != "" {
		if _, err := os.Stat(filepath.Join(*dir, "notes.db")); err != nil {
			return *dir, true, fmt.Errorf("源笔记库不存在：%w", err)
		}
		store, err := openStore(*dir)
		if err != nil {
			return *dir, true, err
		}
		defer store.Close()
		err = store.ExportGit(*export)
		if err == nil {
			fmt.Fprintln(out, "已导出文本快照：", *export)
		}
		return *dir, true, err
	}
	return *dir, false, nil
}
func (a *app) exportGitLibrary() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择文本快照保存位置", Message: "将在所选文件夹内创建或更新 tree-data。", Directory: true, CreateDirectories: true})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	if err = a.flush(); err != nil {
		a.report(err)
		return
	}
	path := filepath.Join(paths[0], "tree-data")
	if err = a.store.ExportGit(path); err != nil {
		a.report(err)
		return
	}
	a.status = "文本快照已保存：" + path
}
func (a *app) restoreGitLibrary() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择 Tree 文本快照", Message: "选择包含 format.json 的 tree-data 文件夹。", Directory: true})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择新笔记库的空文件夹", Directory: true, CreateDirectories: true})
	if err != nil {
		a.report(err)
		return
	}
	if len(dest) == 0 {
		return
	}
	if err = RestoreGit(paths[0], dest[0]); err != nil {
		a.report(err)
		return
	}
	a.status = "笔记库已还原：" + dest[0]
	_, err = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageInfo, Title: "笔记库已还原", Message: "已创建新的本地笔记库", Detail: dest[0]})
	a.report(err)
}
