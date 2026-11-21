package main

import (
	"encoding/json"
	"fmt"
	"os"

	"duanjuapp/native/core"
)

func main() {
	dir := os.Args[1]
	initReq, _ := json.Marshal(map[string]any{"action": "initialize", "directory": dir, "session": "p1", "sequence": 1})
	core.NativeRequest(string(initReq))
	seq := 2
	for _, spec := range os.Args[2:] {
		var input map[string]any
		if json.Unmarshal([]byte(spec), &input) != nil {
			continue
		}
		input["directory"] = dir
		input["session"] = "p1"
		input["sequence"] = seq
		seq++
		raw, _ := json.Marshal(input)
		fmt.Println(core.NativeRequest(string(raw)))
	}
}
