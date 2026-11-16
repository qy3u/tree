# Tree · 本地笔记

使用 [MyGo](https://github.com/egoist/mygo) 原生 UI、原生富文本编辑器和 SQLite 实现的轻量级桌面笔记应用（当前面向 macOS Apple Silicon）。

## 运行与构建

直接打开 `build/darwin-arm64/Tree.app`，或从源码运行（需 Go 1.27.3+）：

```sh
# 开发运行
go tool mygo dev

# 构建本机应用
go tool mygo build -skip-dmg

# 运行测试与检查
go test ./...
go vet ./...
```

## 核心功能

- **页面与标签页**：父子页面树、拖拽排序、页面彩色图标、回收站、Library（最近访问与收藏）、多标签页独立历史与会话恢复。
- **富文本编辑**：Markdown 快捷输入、`/` 命令菜单、六点段落手柄与拖拽排序、选中浮动格式栏，支持表格、图片粘贴、LaTeX 行内/块级公式及多语言代码块高亮。
- **Leaf 知识单元**：通过 `/leaf` 创建独立知识单元，支持折叠预览、原位编辑、锁定保护与整体移动/复制。
- **块引用与页面引用**：复制块链接或页面链接并在正文粘贴为可读引用，点击跳转并定位高亮。
- **计时待办**：通过 `/ttodo` 创建可计时的待办任务，支持预计耗时、右上角悬浮胶囊快速控制/居中跳转，以及每 20 分钟跟随鼠标所在屏幕的悬浮确认提醒。
- **搜索与自动保存**：全局搜索标题、路径与正文；编辑停止后自动保存到本地 SQLite，关闭窗口隐藏到后台，`⌘,` 呼出，`⌘Q` 退出。
- **导入、导出与备份**：支持 Markdown 导入/导出、完整数据库与附件备份，以及 Git 友好的确定性文本快照导出与还原。

## 常用快捷键

| 快捷键 | 操作 |
| --- | --- |
| `⌘,` | 全局呼出窗口 |
| `⌘T` / `⌘W` | 新建标签页 / 关闭标签页（或隐藏到后台） |
| `⌘N` / `⇧⌘N` | 新建顶层页面 / 新建子页面 |
| `⌘P` | 搜索页面 |
| `⌘S` | 立即保存 |
| `⌘B` / `⌘I` / `⌘K` | 加粗 / 斜体 / 编辑链接 |
| `⌘Z` / `⇧⌘Z` | 撤销 / 重做 |

## 本地数据与 Git 文本快照

默认数据目录为 `~/Library/Application Support/Tree Notes/`（包含 `notes.db` 与 `assets/`），可通过 `TREE_NOTES_DATA_DIR` 或 `--data-dir` 指定独立目录：

```sh
# 导出 Git 文本快照（tree-data/）
./build/darwin-arm64/Tree.app/Contents/MacOS/Tree --export-git /path/to/repo/tree-data

# 从文本快照还原到新笔记库
./build/darwin-arm64/Tree.app/Contents/MacOS/Tree --restore-git /path/to/repo/tree-data --data-dir /path/to/library
```

## 设计文档

- [Git 友好序列化方案与格式说明](docs/git-serialization-design.md)
- [Leaf 设计与操作](docs/leaf-design.md)
