package main

import "strings"

func (a *app) pagePath(p *Page) string {
	var parts []string
	for p != nil {
		parts = append([]string{pageTitle(p)}, parts...)
		p = a.find(p.Parent)
	}
	return strings.Join(parts, " / ")
}
func textMatch(text, query string) (int, int) {
	r := []rune(text)
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return -1, -1
	}
	for i := 0; i+len(q) <= len(r); i++ {
		if strings.ToLower(string(r[i:i+len(q)])) == string(q) {
			return i, i + len(q)
		}
	}
	return -1, -1
}
func matchSnippet(text, query string) string {
	r := []rune(strings.ReplaceAll(text, "\n", " "))
	start, end := textMatch(string(r), query)
	if start < 0 {
		start = 0
		end = 0
	}
	lo, hi := max(0, start-24), min(len(r), end+60)
	s := string(r[lo:hi])
	if lo > 0 {
		s = "…" + s
	}
	if hi < len(r) {
		s += "…"
	}
	return s
}
func (a *app) openSearchResult(id, query string) {
	a.selectPage(id)
	a.searchOpen = false
	d := a.current()
	if d == nil {
		return
	}
	start, end := textMatch(d.doc.Text().String(), query)
	if start < 0 {
		for pos, para := range d.doc.Text().Paragraphs() {
			for _, run := range para.Runs {
				at := run.Attributes.Attachment
				if at != nil && strings.HasPrefix(at.URL, "table:") {
					t := d.page.Body.Tables[strings.TrimPrefix(at.URL, "table:")]
					if x, _ := textMatch(t.markdown(), query); x >= 0 {
						start, end = pos, pos+1
					}
				}
			}
		}
	}
	if start >= 0 {
		d.doc.SetSelection(start, end)
		a.editorHandle.Focus()
	} else {
		a.titleHandle.Focus()
	}
}
