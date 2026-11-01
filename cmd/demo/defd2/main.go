// Command defd2 writes two d2 files per workflow definition the demo
// registers, <type>.d2 drawn across and <type>.down.d2 drawn down. The
// docs build renders them with the d2 CLI and the demo app embeds the
// pictures, showing the one that fits the screen.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	_ "git.bytestone.uk/hum3/gobank-workflow/cmd/demo/definitions"
	"git.bytestone.uk/hum3/gobank-workflow/diagram"
)

func main() {
	out := flag.String("out", ".", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, d := range wf.Definitions {
		for suffix, dir := range map[string]diagram.Direction{"": diagram.Across, ".down": diagram.Down} {
			path := filepath.Join(*out, string(d.Type)+suffix+".d2")
			if err := os.WriteFile(path, []byte(diagram.Definition(d, dir, nil)), 0o644); err != nil {
				log.Fatal(err)
			}
			fmt.Println(path)
		}
	}
}
