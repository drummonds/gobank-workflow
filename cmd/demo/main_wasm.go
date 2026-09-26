//go:build js && wasm

package main

import (
	"strings"

	"git.bytestone.uk/hum3/lofigui"
	wasmhttp "github.com/nlepage/go-wasm-http-server/v2"
)

func main() {
	// The service-worker scope becomes <base href>, so every relative
	// link and form action stays inside the scope.
	base := strings.TrimSuffix(lofigui.WASMScopePath(), "/") + "/"
	app, err := NewApp(Options{})
	if err != nil {
		panic(err)
	}
	if _, err := wasmhttp.Serve(app.Handler(base)); err != nil {
		panic(err)
	}
	select {}
}
