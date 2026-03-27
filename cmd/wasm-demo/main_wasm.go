//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/drummonds/lofigui"
)

var state = NewWasmState()

func goRender(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildDashboardHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goAdvanceDay(this js.Value, args []js.Value) any {
	state.AdvanceDays(1)
	return nil
}

func goAdvanceWeek(this js.Value, args []js.Value) any {
	state.AdvanceDays(7)
	return nil
}

func goAdvanceMonth(this js.Value, args []js.Value) any {
	state.AdvanceDays(30)
	return nil
}

func goReset(this js.Value, args []js.Value) any {
	state.Reset()
	return nil
}

func main() {
	js.Global().Set("goRender", js.FuncOf(goRender))
	js.Global().Set("goAdvanceDay", js.FuncOf(goAdvanceDay))
	js.Global().Set("goAdvanceWeek", js.FuncOf(goAdvanceWeek))
	js.Global().Set("goAdvanceMonth", js.FuncOf(goAdvanceMonth))
	js.Global().Set("goReset", js.FuncOf(goReset))
	js.Global().Call("wasmReady")
	<-make(chan struct{})
}
