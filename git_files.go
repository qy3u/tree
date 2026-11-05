package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/sqlite"
)

func readGitDirectory(dir string) (map[string][]byte, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("请选择普通的文本快照目录")
	}
	files := map[string][]byte{}
	var total int64
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("快照中不允许符号链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		// A snapshot is a dedicated subtree, never a whole Git repository.
		if strings.HasPrefix(rel, ".git"+string(filepath.Separator)) || rel == ".git" {
			return errors.New("请选择仓库内的 tree-data 子目录，不要选择仓库根目录")
		}
		if len(files) >= 100000 {
			return errors.New("快照文件数超过 100000")
		}
		data, err := readGitRegular(path)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > 2<<30 {
			return errors.New("快照超过 2 GiB")
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}
func canonicalGitPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve existing ancestors so a symlink cannot hide a data-dir overlap.
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}
func pathsOverlap(a, b string) bool {
	inside := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return inside(a, b) || inside(b, a)
}
func sameGitFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for path, data := range a {
		other, ok := b[path]
		if !ok || !bytes.Equal(data, other) {
			return false
		}
	}
	return true
}
func writeGitStage(parent string, files map[string][]byte) (string, error) {
	dir, err := os.MkdirTemp(parent, ".tree-stage-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	for path, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return "", err
		}
		f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		errClose := f.Close()
		if err == nil {
			err = errClose
		}
		if err != nil {
			return "", err
		}
	}
	ok = true
	return dir, nil
}

// ExportGit replaces only an owned snapshot directory, never a repository
// root or an unrelated directory. It leaves all files untouched on a no-op.
func (s *Store) ExportGit(dir string) error {
	dest, err := canonicalGitPath(dir)
	if err != nil {
		return err
	}
	source, err := filepath.EvalSymlinks(s.dir)
	if err != nil {
		return err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	if pathsOverlap(source, dest) {
		return errors.New("文本快照目录不能与正在使用的笔记数据目录重叠")
	}
	snapshot, err := s.gitSnapshot()
	if err != nil {
		return err
	}
	files, err := snapshotFiles(snapshot)
	if err != nil {
		return err
	}
	existing := false
	if _, err = os.Lstat(dest); err == nil {
		old, err := readGitDirectory(dest)
		if err != nil {
			return err
		}
		if len(old) > 0 {
			previous, err := decodeGitFiles(old)
			if err != nil {
				return fmt.Errorf("目标不是完整的 Tree 文本快照，未覆盖：%w", err)
			}
			if previous.Format.LibraryID != snapshot.Format.LibraryID {
				return errors.New("目标属于另一个笔记库，未覆盖")
			}
			if sameGitFiles(old, files) {
				return nil
			}
		}
		existing = true
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := writeGitStage(filepath.Dir(dest), files)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// Validate the actual staged bytes before changing the previous snapshot.
	staged, err := readGitDirectory(stage)
	if err != nil {
		return err
	}
	if _, err = decodeGitFiles(staged); err != nil {
		return err
	}
	if !existing {
		return os.Rename(stage, dest)
	}
	old, err := os.MkdirTemp(filepath.Dir(dest), ".tree-previous-")
	if err != nil {
		return err
	}
	if err = os.Remove(old); err != nil {
		return err
	}
	if err = os.Rename(dest, old); err != nil {
		return err
	}
	if err = os.Rename(stage, dest); err != nil {
		if rollback := os.Rename(old, dest); rollback != nil {
			return fmt.Errorf("发布失败：%v；旧快照仍在 %s：%w", err, old, rollback)
		}
		return err
	}
	return os.RemoveAll(old)
}

// RestoreGit creates a new library in an absent or empty directory. Existing
// note databases are never overwritten or merged, and IDs remain unchanged.
func RestoreGit(source, destination string) error {
	src, err := canonicalGitPath(source)
	if err != nil {
		return err
	}
	dest, err := canonicalGitPath(destination)
	if err != nil {
		return err
	}
	if pathsOverlap(src, dest) {
		return errors.New("源快照与还原目录不能重叠")
	}
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("还原目标必须是空目录")
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return errors.New("还原目标非空，现有数据未修改")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	files, err := readGitDirectory(src)
	if err != nil {
		return err
	}
	snapshot, err := decodeGitFiles(files)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".tree-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	store, err := openStore(stage)
	if err != nil {
		return err
	}
	defer store.Close()
	var statements []sqlite.Statement
	for _, p := range snapshot.Pages {
		raw, err := json.Marshal(p.Body)
		if err != nil {
			return err
		}
		statements = append(statements, sqlite.Statement{SQL: `INSERT INTO pages(id,parent,title,position,body,revision,updated,deleted) VALUES(?,?,?,?,?,1,?,?)`, Args: []any{p.ID, p.Parent, p.Title, p.Position, string(raw), p.Updated, p.Deleted}})
	}
	snapshot.Settings["libraryID"] = snapshot.Format.LibraryID
	for k, v := range snapshot.Settings {
		statements = append(statements, sqlite.Statement{SQL: `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, Args: []any{k, v}})
	}
	for id, a := range snapshot.Assets {
		statements = append(statements, sqlite.Statement{SQL: `INSERT INTO assets(id,name,path) VALUES(?,?,?)`, Args: []any{id, a.Name, a.Path}})
	}
	for path, data := range snapshot.Binary {
		if err = os.WriteFile(filepath.Join(stage, "assets", path), data, 0600); err != nil {
			return err
		}
	}
	if _, err = store.db.Transaction(context.Background(), statements); err != nil {
		return err
	}
	// Verify the reconstructed DB through the real exporter before publishing.
	restored, err := store.gitSnapshot()
	if err != nil {
		return err
	}
	delete(snapshot.Settings, "libraryID")
	expected, err := snapshotFiles(snapshot)
	if err != nil {
		return err
	}
	actual, err := snapshotFiles(restored)
	if err != nil {
		return err
	}
	if !sameGitFiles(expected, actual) {
		return errors.New("还原校验失败：重新序列化后内容不一致")
	}
	if err = store.Close(); err != nil {
		return err
	}
	// Only an empty destination can be removed here; a concurrent new file
	// prevents publication rather than being overwritten.
	if err = os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, dest)
}
