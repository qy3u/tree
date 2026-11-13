package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Each knowledge unit has its own file. Its editable content stays structured
// and line-diffable; view state and measured card sizes are never exported.
type gitLeaf struct {
	Locked bool                `json:"locked,omitempty"`
	Title  string              `json:"title"`
	Order  []string            `json:"block_order"`
	Blocks map[string]gitBlock `json:"blocks"`
	Times  []gitTime           `json:"time,omitempty"`
}

func exportGitLeaf(id string, leaf Leaf) (gitLeaf, error) {
	out := gitLeaf{Locked: leaf.Locked, Title: leaf.Title, Order: []string{}, Blocks: map[string]gitBlock{}}
	paras, err := storedParagraphs(leaf.Content.Text)
	if err != nil {
		return out, err
	}
	for _, p := range paras {
		key := p.Style.ID
		if !gitName(key) {
			return out, errors.New("Leaf 内容缺少稳定 ID")
		}
		if _, ok := out.Blocks[key]; ok {
			return out, errors.New("Leaf 内容 ID 重复")
		}
		for _, r := range p.Runs {
			if r.Attachment != nil && strings.HasPrefix(r.Attachment.URL, "leaf:") {
				return out, errors.New("不支持嵌套 Leaf")
			}
		}
		b, err := exportGitBlock(p, leaf.Content)
		if err != nil {
			return out, err
		}
		out.Order = append(out.Order, key)
		out.Blocks[key] = b
		if p.Style.Timer.Enabled() {
			for _, seg := range leaf.Content.TaskTimes[p.Style.Timer.HistoryID] {
				seg.StartedAt = seg.StartedAt.UTC()
				seg.EndedAt = seg.EndedAt.UTC()
				out.Times = append(out.Times, gitTime{gitTimeID(id, key, seg), id, key, seg})
			}
		}
	}
	slices.SortFunc(out.Times, func(a, b gitTime) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func importGitLeaf(id string, in gitLeaf, assets map[string]gitAsset) (Leaf, error) {
	leaf := Leaf{Locked: in.Locked, Title: in.Title, Content: emptyBody()}
	if len(in.Order) == 0 || len(in.Order) != len(in.Blocks) {
		return leaf, errors.New("Leaf 内容顺序无效")
	}
	leaf.Content.TaskTimes = map[string][]TaskTimeSegment{}
	seen := map[string]bool{}
	doc := storedText{}
	for _, key := range in.Order {
		b, ok := in.Blocks[key]
		if !ok || seen[key] {
			return leaf, errors.New("Leaf 内容缺失或重复")
		}
		seen[key] = true
		p, err := importGitBlock(key, b, &leaf.Content)
		if err != nil {
			return leaf, err
		}
		for _, r := range p.Runs {
			if at := r.Attachment; at != nil {
				if strings.HasPrefix(at.URL, "leaf:") {
					return leaf, errors.New("不支持嵌套 Leaf")
				}
				if strings.HasPrefix(at.URL, "asset:") {
					if _, ok := assets[strings.TrimPrefix(at.URL, "asset:")]; !ok {
						return leaf, errors.New("Leaf 引用了缺失的附件")
					}
				}
			}
		}
		doc.Paragraphs = append(doc.Paragraphs, p)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return leaf, err
	}
	if err = json.Unmarshal(raw, &leaf.Content.Text); err != nil {
		return leaf, err
	}
	page := &Page{ID: id, Body: leaf.Content}
	if err = validateGitTimes([]*Page{page}, in.Times); err != nil {
		return leaf, err
	}
	leaf.Content = page.Body
	return leaf, nil
}

func validateStoredBody(raw []byte) error {
	var body struct {
		Icon      string                       `json:"icon"`
		Schema    int                          `json:"schema"`
		Text      json.RawMessage              `json:"text"`
		Tables    map[string]Table             `json:"tables"`
		TaskTimes map[string][]TaskTimeSegment `json:"taskTimes"`
		Leaves    map[string]struct {
			Title   string          `json:"title"`
			Content json.RawMessage `json:"content"`
		} `json:"leaves"`
	}
	if err := strictJSON(raw, &body); err != nil {
		return err
	}
	if body.Schema != 1 && body.Schema != 2 {
		return errors.New("不支持的正文版本")
	}
	var text storedText
	if err := strictJSON(body.Text, &text); err != nil {
		return err
	}
	for id, leaf := range body.Leaves {
		if !gitName(id) {
			return fmt.Errorf("无效 Leaf ID %q", id)
		}
		if err := validateStoredBody(leaf.Content); err != nil {
			return err
		}
	}
	return nil
}
