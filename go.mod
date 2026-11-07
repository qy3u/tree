module tree.local/notes

go 1.27.3

require (
	github.com/caelyreth/untie v0.5.0
	github.com/caelyreth/untie/contrib/mygo v0.5.0
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/go-typeset/bidi v0.3.1 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/egoist/mygo v0.7.4
	github.com/egoist/mygo/treerichtext v0.0.0
)

replace github.com/egoist/mygo/treerichtext => ./internal/treerichtext
