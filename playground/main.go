//go:build js && wasm

// Command playground is cligram for the browser: built to WebAssembly, it
// gives the page one function, cligramDraw, which takes a request as JSON
// and returns the drawing, or what is wrong with the source, as JSON.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/isacikgoz/cligram/internal/draw"
)

// result is what the page gets back: a drawing, or an error.
type result struct {
	*draw.Drawing
	Error string `json:"error,omitempty"`
}

func main() {
	js.Global().Set("cligramDraw", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var r draw.Request
		if len(args) == 0 || json.Unmarshal([]byte(args[0].String()), &r) != nil {
			return encode(result{Error: "the request is not JSON"})
		}
		d, err := draw.Draw(r)
		if err != nil {
			return encode(result{Error: err.Error()})
		}
		return encode(result{Drawing: d})
	}))
	select {} // serve calls until the page goes away
}

func encode(r result) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"error":"cannot encode the drawing"}`
	}
	return string(b)
}
