//go:build !(js && wasm)

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func portOf(t *testing.T, ln net.Listener) int {
	t.Helper()
	return ln.Addr().(*net.TCPAddr).Port
}

func holdPort(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln, fmt.Sprintf(":%d", portOf(t, ln))
}

func TestListenDemo_FreePortBindsAsAsked(t *testing.T) {
	probe, addr := holdPort(t)
	want := portOf(t, probe)
	probe.Close()

	ln, err := listenDemo(addr, strings.NewReader(""), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got := portOf(t, ln); got != want {
		t.Fatalf("bound %d, want %d", got, want)
	}
}

func TestListenDemo_BusyPortNonInteractiveMovesToNextFree(t *testing.T) {
	held, addr := holdPort(t)
	var out bytes.Buffer

	ln, err := listenDemo(addr, strings.NewReader(""), &out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if portOf(t, ln) == portOf(t, held) {
		t.Fatal("bound the held port")
	}
	if !strings.Contains(out.String(), addr) {
		t.Fatalf("message should name the busy port %s:\n%s", addr, out.String())
	}
}

func TestListenDemo_BusyPortInteractiveNextOnRequest(t *testing.T) {
	held, addr := holdPort(t)

	ln, err := listenDemo(addr, strings.NewReader("n\n"), &bytes.Buffer{}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if portOf(t, ln) == portOf(t, held) {
		t.Fatal("bound the held port")
	}
}

func TestListenDemo_BusyPortInteractiveQuit(t *testing.T) {
	_, addr := holdPort(t)
	if _, err := listenDemo(addr, strings.NewReader("q\n"), &bytes.Buffer{}, true); err == nil {
		t.Fatal("quit should return an error, not a listener")
	}
}

func TestListenDemo_BusyPortInteractiveKillFreesPort(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperHoldPort")
	cmd.Env = append(os.Environ(), "DEMO_HELPER_HOLD_PORT=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Reap promptly so the child is gone, not a zombie, once killed.
	go cmd.Wait()
	t.Cleanup(func() { cmd.Process.Kill() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr := ":" + strings.TrimSpace(line)

	var out bytes.Buffer
	ln, err := listenDemo(addr, strings.NewReader("k\n"), &out, true)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	defer ln.Close()
	if got := ":" + fmt.Sprint(portOf(t, ln)); got != addr {
		t.Fatalf("bound %s, want the freed %s", got, addr)
	}
	if !strings.Contains(out.String(), fmt.Sprint(cmd.Process.Pid)) {
		t.Fatalf("prompt should name the holder pid %d:\n%s", cmd.Process.Pid, out.String())
	}
}

// TestHelperHoldPort is the child of the kill test: it holds a port and
// prints it, then waits to be killed.
func TestHelperHoldPort(t *testing.T) {
	if os.Getenv("DEMO_HELPER_HOLD_PORT") == "" {
		t.Skip("helper process")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(portOf(t, ln))
	time.Sleep(time.Minute)
}

func TestPortHolder_FindsOwnListener(t *testing.T) {
	held, _ := holdPort(t)
	pid, err := portHolder(portOf(t, held))
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Fatalf("holder pid %d, want %d", pid, os.Getpid())
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("holder pid %d should be live: %v", pid, err)
	}
}
