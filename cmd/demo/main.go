//go:build !(js && wasm)

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
	ln, err := listenDemo(*addr, os.Stdin, os.Stdout, isTerminal(os.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gobank-workflow demo %s at http://localhost:%d\n", version, ln.Addr().(*net.TCPAddr).Port)
	log.Fatal(http.Serve(ln, app.Handler("/")))
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
