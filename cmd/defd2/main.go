// Command defd2 writes one d2 file per workflow definition, generated from
// the engine's registry. The docs build renders them with the d2 CLI.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	wf "git.bytestone.uk/hum3/gobank-workflow"
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
		if err := os.WriteFile(path, []byte(diagram.Definition(d)), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
	}
}
