package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestLeafLockTogglePersistenceAndEditing(t *testing.T) {
	a, tt, d, v := createTestLeaf(t)
	tt.Type("知识标题")
	tt.Key(0, ui.KeyEnter)
	tt.Type("保留正文")
	if err := tt.Click("锁定 Leaf"); err != nil {
		t.Fatal(err)
	}
	id := v.draft.leafID
	if !d.page.Body.Leaves[id].Locked {
		t.Fatal("not locked")
	}
	if _, ok := tt.Find("Leaf 标题"); ok {
		t.Fatal("locked title remains editable")
	}
	if err := tt.Click("Leaf 正文"); err != nil {
		t.Fatal(err)
	}
	tt.Type("不能写入")
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyZ)
	if v.draft.doc.Text().String() != "保留正文" {
		t.Fatalf("locked content changed: %q", v.draft.doc.Text().String())
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.find(d.page.ID).Body.Leaves[id].Locked {
		t.Fatal("lost persisted lock")
	}
	exported, err := exportGitLeaf(id, d.page.Body.Leaves[id])
	if err != nil {
		t.Fatal(err)
	}
	restored, err := importGitLeaf(id, exported, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Locked {
		t.Fatal("Git round trip lost lock")
	}
	if err := tt.Click("解锁 Leaf"); err != nil {
		t.Fatal(err)
	}
	if d.page.Body.Leaves[id].Locked {
		t.Fatal("still locked")
	}
	if err := tt.Click("Leaf 正文"); err != nil {
		t.Fatal(err)
	}
	before := v.draft.doc.Version()
	tt.Type("继续编辑")
	if v.draft.doc.Version() == before {
		t.Fatal("unlock did not restore editing")
	}
	a.syncLeaf(v.draft)
	if d.page.Body.Leaves[id].Locked {
		t.Fatal("sync changed lock")
	}
}
