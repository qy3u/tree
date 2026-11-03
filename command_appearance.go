package main

func commandGroup(id string) string {
	switch id {
	case "image", "paste-image", "code", "table":
		return "媒体与其他"
	}
	return "基础内容"
}
func commandAppearance(cmd blockCommand) (icon, hint, label string) {
	label = cmd.Label
	switch cmd.ID {
	case "text":
		return "text", "", label
	case "h1":
		return "h1", "#", label
	case "h2":
		return "h2", "##", label
	case "h3":
		return "h3", "###", label
	case "page":
		return "page", "", label
	case "bullet":
		return "bullet", "-", label
	case "number":
		return "number", "1.", label
	case "todo":
		return "todo", "[]", label
	case "quote":
		return "quote", ">", label
	case "divider":
		return "divider", "---", label
	case "image":
		return "image", "", "图片"
	case "paste-image":
		return "paste", "", label
	case "code":
		return "code", "```", label
	case "table":
		return "table", "", "表格"
	}
	return "page", "", label
}
