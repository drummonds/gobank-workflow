// Command defd2 writes one d2 file per workflow definition the demo
// registers. The docs build renders them with the d2 CLI and the demo app
// embeds the pictures.
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
		path := filepath.Join(*out, string(d.Type)+".d2")
		if err := os.WriteFile(path, []byte(diagram.Definition(d, nil)), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
	}
}
