package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

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
		if input["action"] == "contractCheck" {
			reports := core.RunSourceContracts(context.Background(), 5*time.Second)
			rawReports, _ := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"reports": reports}})
			fmt.Println(string(rawReports))
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
