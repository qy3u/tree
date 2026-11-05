package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

const gitFormatVersion = 1

type gitFormat struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	LibraryID string `json:"library_id"`
}
type gitPage struct {
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Position int64  `json:"position"`
	Icon     string `json:"icon,omitempty"`
	Updated  string `json:"updated"`
	Deleted  string `json:"deleted,omitempty"`
}

// The native clipboard JSON is used only as an adapter. The public format
// does not expose clipboard IDs, table versions, or timer history pointers.
type storedText struct {
	Paragraphs []storedParagraph `json:"paragraphs"`
}
type storedParagraph struct {
	Style richtext.ParagraphStyle `json:"style"`
	Runs  []storedRun             `json:"runs,omitempty"`
}
type storedRun struct {
	Text       string         `json:"text"`
	Attributes gitAttributes  `json:"attributes"`
	Attachment *gitAttachment `json:"attachment,omitempty"`
}
type gitAttributes struct {
	Font          string  `json:"font,omitempty"`
	Size          float32 `json:"size,omitempty"`
	Weight        int     `json:"weight,omitempty"`
	Italic        bool    `json:"italic,omitempty"`
	Underline     bool    `json:"underline,omitempty"`
	Strikethrough bool    `json:"strikethrough,omitempty"`
	Color         string  `json:"color,omitempty"`
	Background    string  `json:"background,omitempty"`
	Code          bool    `json:"code,omitempty"`
	Link          string  `json:"link,omitempty"`
	Tag           string  `json:"tag,omitempty"`
}
type gitAttachment struct {
	Decorative bool    `json:"decorative,omitempty"`
	Name       string  `json:"name,omitempty"`
	URL        string  `json:"url"`
	Width      float32 `json:"width"`
	Height     float32 `json:"height"`
	Table      *Table  `json:"table,omitempty"`
}
type gitRun struct {
	Text       *string        `json:"text,omitempty"`
	Lines      []string       `json:"lines,omitempty"`
	Attributes gitAttributes  `json:"attributes,omitempty"`
	Attachment *gitAttachment `json:"attachment,omitempty"`
}
type gitBlock struct {
	Type  string                  `json:"type"`
	Style richtext.ParagraphStyle `json:"style"`
	Runs  []gitRun                `json:"runs"`
	Timer *richtext.TaskTimer     `json:"timer,omitempty"`
}
type gitTime struct {
	ID    string `json:"id"`
	Page  string `json:"page"`
	Block string `json:"block"`
	TaskTimeSegment
}
type gitAsset struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type gitSnapshot struct {
	Format   gitFormat
	Pages    []*Page
	Settings map[string]string
	Assets   map[string]gitAsset
	Binary   map[string][]byte
}

func gitJSON(v any, pretty bool) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if pretty {
		e.SetIndent("", "  ")
	}
	err := e.Encode(v)
	return b.Bytes(), err
}

// Reject duplicate keys as well as unknown fields: encoding/json alone
// silently accepts duplicates, which would turn a hand-edited file into loss.
func strictJSON(data []byte, out any) error {
	if !utf8.Valid(data) {
		return errors.New("JSON 不是有效 UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func() error
	value = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		if token == json.Delim('{') {
			keys := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return fmt.Errorf("重复或无效字段：%v", key)
				}
				keys[name] = true
				if err = value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		if token == json.Delim('[') {
			for d.More() {
				if err := value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		if _, ok := token.(json.Delim); ok {
			return errors.New("无效 JSON")
		}
		return nil
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("JSON 后有多余内容")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	return d.Decode(out)
}
func gitName(s string) bool {
	if s == "" || len(s) > 200 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return s[0] != '.'
}
func gitHash(parts ...string) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func assetHash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func gitTimeID(page, block string, segment TaskTimeSegment) string {
	return gitHash(page, block, segment.StartedAt.UTC().Format(time.RFC3339Nano))
}
func gitBlockType(s richtext.ParagraphStyle) string {
	switch {
	case s.Timer.Enabled():
		return "timed_todo"
	case s.CodeBlock:
		return "code"
	case s.Heading > 0:
		return "heading"
	case s.List == richtext.Tasks:
		return "todo"
	case s.List == richtext.Bullets:
		return "bullet"
	case s.List == richtext.Numbers:
		return "number"
	case s.Quote:
		return "quote"
	default:
		return "text"
	}
}
func storedParagraphs(text richtext.Text) ([]storedParagraph, error) {
	raw, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	var doc storedText
	if err = strictJSON(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Paragraphs, nil
}
func exportGitBlock(p storedParagraph, body Body) (gitBlock, error) {
	b := gitBlock{Type: gitBlockType(p.Style), Style: p.Style, Runs: []gitRun{}}
	b.Style.ID = ""
	b.Style.Timer = richtext.TaskTimer{}
	if p.Style.Timer.Enabled() {
		timer := p.Style.Timer
		timer.HistoryID = ""
		b.Timer = &timer
	}
	// Canonicalize adjacent plain runs without changing attachment boundaries.
	var runs []storedRun
	for _, r := range p.Runs {
		if len(runs) > 0 && r.Attachment == nil && runs[len(runs)-1].Attachment == nil && r.Attributes == runs[len(runs)-1].Attributes {
			runs[len(runs)-1].Text += r.Text
		} else {
			runs = append(runs, r)
		}
	}
	for _, r := range runs {
		g := gitRun{Attributes: r.Attributes, Attachment: r.Attachment}
		if p.Style.CodeBlock {
			g.Lines = strings.Split(r.Text, "\u2028")
		} else {
			text := r.Text
			g.Text = &text
		}
		if r.Attachment != nil && strings.HasPrefix(r.Attachment.URL, "table:") {
			table, ok := body.Tables[strings.TrimPrefix(r.Attachment.URL, "table:")]
			if !ok {
				return b, errors.New("缺少当前表格数据")
			}
			at := *r.Attachment
			at.URL = "table:"
			at.Name = "表格"
			at.Table = &table
			g.Attachment = &at
		}
		b.Runs = append(b.Runs, g)
	}
	return b, nil
}
func importGitBlock(id string, b gitBlock, body *Body) (storedParagraph, error) {
	p := storedParagraph{Style: b.Style, Runs: []storedRun{}}
	if !gitName(id) || p.Style.ID != "" || p.Style.Timer != (richtext.TaskTimer{}) {
		return p, errors.New("Block ID 或内部状态字段无效")
	}
	if p.Style.Align > ui.End {
		return p, errors.New("不支持的段落对齐方式")
	}
	p.Style.ID = id
	if b.Timer != nil {
		if b.Timer.HistoryID != "" || !b.Timer.Enabled() {
			return p, errors.New("计时状态无效")
		}
		p.Style.Timer = *b.Timer
	}
	if gitBlockType(p.Style) != b.Type {
		return p, fmt.Errorf("Block 类型与格式不一致：%s", b.Type)
	}
	for i, r := range b.Runs {
		var text string
		if p.Style.CodeBlock {
			if r.Text != nil || len(r.Lines) == 0 {
				return p, errors.New("代码必须使用非空 lines 数组")
			}
			for _, line := range r.Lines {
				if strings.ContainsAny(line, "\n\u2028") {
					return p, errors.New("代码行包含未拆分的换行")
				}
			}
			text = strings.Join(r.Lines, "\u2028")
		} else {
			if r.Text == nil || r.Lines != nil {
				return p, errors.New("正文必须使用 text")
			}
			text = *r.Text
		}
		if strings.Contains(text, "\n") {
			return p, errors.New("Block 内包含段落分隔符")
		}
		at := r.Attachment
		if at != nil {
			if text != string(richtext.ObjectReplacement) || at.Width < 1 || at.Height < 1 {
				return p, errors.New("附件占位符或尺寸无效")
			}
			copy := *at
			at = &copy
			if at.URL == "table:" {
				if at.Table == nil || len(at.Table.Cells) == 0 {
					return p, errors.New("表格内容缺失")
				}
				key := "git-" + gitHash(id, fmt.Sprint(i))
				body.Tables[key] = *at.Table
				at.URL = "table:" + key
				at.Table = nil
			} else if at.Table != nil || strings.HasPrefix(at.URL, "table:") {
				return p, errors.New("无效的表格引用")
			}
		}
		p.Runs = append(p.Runs, storedRun{Text: text, Attributes: r.Attributes, Attachment: at})
	}
	// Native decoding normalizes invalid styles/links/colors. Reject changes
	// instead of silently losing fields in a successful-looking restore.
	raw, err := json.Marshal(storedText{[]storedParagraph{p}})
	if err != nil {
		return p, err
	}
	var text richtext.Text
	if err = json.Unmarshal(raw, &text); err != nil {
		return p, err
	}
	normalized, err := storedParagraphs(text)
	if err != nil {
		return p, err
	}
	if len(normalized) != 1 || normalized[0].Style != p.Style {
		return p, errors.New("不支持或无效的 Block 样式")
	}
	got, err := exportGitBlock(normalized[0], *body)
	if err != nil {
		return p, err
	}
	expected, err := exportGitBlock(p, *body)
	if err != nil {
		return p, err
	}
	if !reflect.DeepEqual(got, expected) {
		return p, errors.New("Block 含无法无损恢复的文字或格式")
	}
	return p, nil
}
func validateGitTimes(pages []*Page, records []gitTime) error {
	byBlock := map[string]*richtext.TaskTimer{}
	bodies := map[string]*Body{}
	for _, p := range pages {
		for _, para := range p.Body.Text.Paragraphs() {
			key := p.ID + "/" + para.Style.ID
			timer := para.Style.Timer
			if timer.Enabled() {
				byBlock[key] = &timer
				bodies[key] = &p.Body
			}
		}
	}
	groups := map[string][]gitTime{}
	seen := map[string]bool{}
	for _, r := range records {
		key := r.Page + "/" + r.Block
		if byBlock[key] == nil || seen[r.ID] || r.ID != gitTimeID(r.Page, r.Block, r.TaskTimeSegment) {
			return errors.New("计时记录重复或找不到所属任务")
		}
		seen[r.ID] = true
		if r.DurationNS <= 0 || !r.EndedAt.After(r.StartedAt) || int64(r.EndedAt.Sub(r.StartedAt)) != r.DurationNS || r.TimeZone == "" || r.UTCOffsetSeconds < -86400 || r.UTCOffsetSeconds > 86400 {
			return errors.New("计时区间或耗时无效")
		}
		loc := time.FixedZone(r.TimeZone, r.UTCOffsetSeconds)
		start, end := r.StartedAt.In(loc), r.EndedAt.In(loc)
		// The stored offset defines the original local date even after travel or
		// time-zone database changes. Midnight is allowed as the exclusive end.
		if start.Format("2006-01-02") != r.Date || end.Add(-time.Nanosecond).Format("2006-01-02") != r.Date {
			// A named DST zone can change offset within this single local day.
			named, err := time.LoadLocation(r.TimeZone)
			if err != nil || r.StartedAt.In(named).Format("2006-01-02") != r.Date || r.EndedAt.Add(-time.Nanosecond).In(named).Format("2006-01-02") != r.Date {
				return errors.New("计时日期与区间不一致")
			}
		}
		groups[key] = append(groups[key], r)
	}
	for key, timer := range byBlock {
		group := groups[key]
		slices.SortFunc(group, func(a, b gitTime) int { return a.StartedAt.Compare(b.StartedAt) })
		var total int64
		segments := []TaskTimeSegment{}
		var end time.Time
		for _, r := range group {
			if r.StartedAt.Before(end) || r.DurationNS > math.MaxInt64-total {
				return errors.New("计时记录重叠或耗时溢出")
			}
			end = r.EndedAt
			total += r.DurationNS
			seg := r.TaskTimeSegment
			seg.StartedAt = seg.StartedAt.UTC()
			seg.EndedAt = seg.EndedAt.UTC()
			segments = append(segments, seg)
		}
		if total != timer.ElapsedNS {
			return fmt.Errorf("任务 %s 的累计耗时与记录不一致", key)
		}
		if timer.State == richtext.TimerRunning && !end.IsZero() && time.Unix(0, timer.StartedAtNS).Before(end) {
			return errors.New("运行中的计时与已保存区间重叠")
		}
		if len(segments) > 0 {
			body := bodies[key]
			history := "git-" + gitHash(key)
			body.TaskTimes[history] = segments
			id := strings.SplitN(key, "/", 2)[1]
			body.Text = body.Text.FormatParagraphs(0, body.Text.Len(), func(st *richtext.ParagraphStyle) {
				if st.ID == id {
					st.Timer.HistoryID = history
				}
			})
		}
	}
	return nil
}
