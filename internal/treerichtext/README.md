# Tree's native rich text adapter

Source: `github.com/egoist/mygo/ui/richtext` at **v0.7.4**, with its original MIT license. This local module uses MyGo's native rendering, text input, layout and clipboard implementation; it is not a WebView or a separate editor engine.

Tree-specific changes:

- `EditorOptions.LeadingSpace`: scrollable space for the editable page title and child-page links.
- `Document.ScrollOffset`, `ScrollBy`: share the document scroll with title and attachment controls.
- `Document.VisibleAttachments`: expose the native attachment layout to inline table and image controls.
- `EditorOptions.OnPasteImage`: import pasted bitmap images into Tree's local asset directory.
- `EditorOptions.BeforeKey`: resolve slash commands against the latest document when typing and Enter arrive before a frame.
- Shift+Enter inserts an inline line break; Tab / Shift+Tab indent and outdent plain paragraphs as well as lists.
- Enter inside code inserts an inline line break instead of splitting the block or leaving it on empty lines; Markdown and HTML export preserve these code newlines.
- Empty-document placeholder follows the native paragraph origin.
- The first code block excludes `LeadingSpace` from its background, keeping page titles and child-page links outside the code fill.

- `ParagraphStyle.CodeLanguage`: persisted fenced-code language, also read/written by Markdown.
- Code highlighting reuses MyGo `ui/editor` tokenizers as display-only spans, carries multiline lexical state across legacy code paragraphs and caches incoming states per document version.
- `EditorOptions.CodeHeaderHeight` and `Document.VisibleCodeBlocks`: reserve and locate native language/copy controls without covering code text or the page title.

- `ParagraphBox.FirstLineCenter`: native first-line metrics for vertically centering the block handle across headings, body text, lists and code.

- A plain left click below a terminal code block appends one ordinary paragraph and focuses it in one undo step; readonly views, selection drags, and clicks inside code retain their existing behavior.

- The painted insertion caret uses natural line font metrics, excluding line leading and centered inside the line; selection, hit testing and IME layout geometry stay unchanged.

- `EditorOptions.OnPasteText`: resolve plain clipboard text to app-owned rich references, retaining the destination paragraph style; code blocks and paste-and-match-style remain literal.

- `EditorOptions.OnPasteRichText` restores app-owned inline reference icons after rich clipboard decoding. Decorative attachments retain metadata in native JSON but are omitted from Markdown/HTML export; reference typography uses the existing `TagStyle` hook.

- Link typing boundaries do not extend an explicitly stored link underline into subsequent plain text; ordinary non-link underlining still extends.

- `EditorOptions.DrawAttachment` lets page-reference icons use live target-page metadata while preserving the underlying text and attachment IDs.

Root-package UI tests exercise these additions. When upgrading MyGo, compare this directory against the new `ui/richtext` source and reapply these changes; do not overwrite it blindly.
