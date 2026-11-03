package main

import "github.com/egoist/mygo/ui"

// A small, consistent set of outline icons, rendered by MyGo at display scale.
var icons = map[string]*ui.SVG{}

func init() {
	paths := map[string]string{
		"library":      `<rect x="3" y="4" width="4" height="16" rx=".5"/><rect x="9" y="4" width="4" height="16" rx=".5"/><path d="m15 5 4-1 4 15-4 1z"/>`,
		"clock":        `<circle cx="12" cy="12" r="9"/><path d="M12 6v6H7"/>`,
		"star":         `<path d="m12 3 2.8 5.7 6.3.9-4.5 4.4 1.1 6.2-5.7-3-5.7 3 1.1-6.2L3 9.6l6.2-.9z"/>`,
		"star-filled":  `<path fill="currentColor" d="m12 3 2.8 5.7 6.3.9-4.5 4.4 1.1 6.2-5.7-3-5.7 3 1.1-6.2L3 9.6l6.2-.9z"/>`,
		"shuffle":      `<path d="M3 6h3l12 12h3M17 14l4 4-4 4M3 18h3l12-12h3M17 2l4 4-4 4"/>`,
		"paragraph":    `<path d="M13 4H9a5 5 0 0 0 0 10h4M13 4v16M18 4v16M10 4h11"/>`,
		"copy":         `<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M15 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h3"/>`,
		"chevron-down": `<path d="m6 9 6 6 6-6"/>`,
		"compose":      `<path d="M13 4H6a3 3 0 0 0-3 3v11a3 3 0 0 0 3 3h11a3 3 0 0 0 3-3v-7M10 14l-1 4 4-1L22 8l-3-3z"/>`,
		"back":         `<path d="m15 5-7 7 7 7"/>`,
		"forward":      `<path d="m9 5 7 7-7 7"/>`,
		"close":        `<path d="m6 6 12 12M18 6 6 18"/>`,
		"text":         `<path d="M5 6V4h14v2M12 4v16M8 20h8"/>`,
		"h1":           `<path d="M3 5v14M13 5v14M3 12h10M17 11l3-2v10"/>`,
		"h2":           `<path d="M2 5v14M12 5v14M2 12h10M16 11c0-4 6-4 6 0 0 2-6 5-6 8h6"/>`,
		"h3":           `<path d="M2 5v14M12 5v14M2 12h10M16 9h6l-4 4c5 0 5 7-2 6"/>`,
		"bullet":       `<circle cx="4" cy="6" r="1"/><circle cx="4" cy="12" r="1"/><circle cx="4" cy="18" r="1"/><path d="M9 6h12M9 12h12M9 18h12"/>`,
		"number":       `<path d="M3 4h1v5M2 9h4M2 14c0-3 4-3 4 0 0 2-4 3-4 5h4M10 6h11M10 12h11M10 18h11"/>`,
		"todo":         `<path d="m3 5 2 2 4-5M12 6h9M12 17h9"/><rect x="3" y="13" width="6" height="6" rx=".5"/>`,
		"quote":        `<path d="M5 5v14M10 8h11M10 13h9M10 18h7"/>`,
		"divider":      `<path d="M3 12h18"/>`,
		"image":        `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8" cy="8" r="1.5"/><path d="m3 17 5-5 4 4 4-6 5 7"/>`,
		"paste":        `<path d="M9 4H5a2 2 0 0 0-2 2v14h18V6a2 2 0 0 0-2-2h-4"/><rect x="8" y="2" width="8" height="5" rx="1"/><path d="m7 16 3-4 3 3 2-2 3 3"/>`,
		"table":        `<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M3 9h18M3 14h18M10 4v16"/>`,
		"page":         `<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6M8 13h8M8 17h6"/>`,
		"search":       `<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/>`,
		"plus":         `<path d="M12 5v14M5 12h14"/>`,
		"trash":        `<path d="M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7"/>`,
		"more":         `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
		"grip":         `<circle cx="9" cy="5" r=".7"/><circle cx="15" cy="5" r=".7"/><circle cx="9" cy="12" r=".7"/><circle cx="15" cy="12" r=".7"/><circle cx="9" cy="19" r=".7"/><circle cx="15" cy="19" r=".7"/>`,
		"link":         `<path d="m10 14 4-4M8 16l-1 1a3.5 3.5 0 0 1-5-5l5-5a3.5 3.5 0 0 1 5 0M16 8l1-1a3.5 3.5 0 0 1 5 5l-5 5a3.5 3.5 0 0 1-5 0"/>`,
		"code":         `<path d="m8 7-5 5 5 5m8-10 5 5-5 5"/>`,
		"bold":         `<path d="M7 4h6a4 4 0 0 1 0 8H7V4zm0 8h7a4 4 0 0 1 0 8H7v-8z"/>`,
		"italic":       `<path d="M10 4h9M5 20h9M15 4 9 20"/>`,
		"strike":       `<path d="M17 6c-2-3-9-3-9 1 0 2 2 3 5 4M7 18c2 3 10 3 10-1 0-1-.5-2-2-3M3 12h18"/>`,
	}
	for name, path := range paths {
		icons[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
	}
}

func iconButton(c *ui.Context, icon, label string) ui.Element {
	return quietButton(c, "").Label(label).Tooltip(label).Size(28, 28).Padding(5).Justify(ui.Center).Children(func() {
		ui.Icon(c, icons[icon]).FontSize(18)
	})
}
