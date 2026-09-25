//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/drummonds/lofigui"

	"git.bytestone.uk/hum3/gobank-workflow/cmd/wasm-demo/pipeline"
)

var state = NewWasmState()

var release = pipeline.New()

func goPipelineRender(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(BuildPipelineHTML(release))
	return js.ValueOf(lofigui.Buffer())
}

// goPipelineAct applies one UI action to the release pipeline and returns
// an error message, or "" on success.
func goPipelineAct(this js.Value, args []js.Value) any {
	var err error
	switch args[0].String() {
	case "start":
		err = release.Start()
	case "next":
		err = release.Next()
	case "end":
		err = release.RunToEnd()
	case "fail":
		release.SetFailNext(!release.FailNext())
	case "reset":
		release.Reset()
	}
	if err != nil {
		return js.ValueOf(err.Error())
	}
	return js.ValueOf("")
}

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
	js.Global().Set("goPipelineRender", js.FuncOf(goPipelineRender))
	js.Global().Set("goPipelineAct", js.FuncOf(goPipelineAct))
	js.Global().Call("wasmReady")
	<-make(chan struct{})
}
