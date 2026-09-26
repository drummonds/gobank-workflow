//go:build !(js && wasm)

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":1360", "listen address")
	accounts := flag.Int("accounts", 0, "savings accounts in the simulated book (default 250)")
	flag.Parse()

	app, err := NewApp(Options{Accounts: *accounts})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	fmt.Printf("gobank-workflow demo %s at http://localhost%s\n", version, *addr)
	log.Fatal(http.ListenAndServe(*addr, app.Handler("/")))
}
