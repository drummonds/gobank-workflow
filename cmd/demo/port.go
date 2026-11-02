//go:build !(js && wasm)

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// listenDemo binds addr. When the port is already taken it reports who holds
// it and, on a terminal, offers to kill the holder or take the next free
// port; without a terminal it takes the next free port and says so.
func listenDemo(addr string, in io.Reader, out io.Writer, interactive bool) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil || !errors.Is(err, syscall.EADDRINUSE) {
		return ln, err
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("listen address %s: %w", addr, err)
	}
	pid, _ := portHolder(port)

	if !interactive {
		fmt.Fprintf(out, "port %s is in use%s; taking the next free port\n", addr, holderNote(pid))
		return nextFree(addr, port)
	}
	fmt.Fprintf(out, "port %s is in use%s.\n", addr, holderNote(pid))
	for {
		if pid > 0 {
			fmt.Fprint(out, "[k]ill it, take the [n]ext free port, or [q]uit? ")
		} else {
			fmt.Fprint(out, "take the [n]ext free port, or [q]uit? ")
		}
		answer, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && answer == "" {
			return nil, fmt.Errorf("port %s is in use; no answer", addr)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "k":
			if pid <= 0 {
				continue
			}
			if err := killAndWait(pid); err != nil {
				return nil, err
			}
			return net.Listen("tcp", addr)
		case "n", "":
			return nextFree(addr, port)
		case "q":
			return nil, fmt.Errorf("port %s is in use", addr)
		}
	}
}

func holderNote(pid int) string {
	if pid <= 0 {
		return ""
	}
	return fmt.Sprintf(" by %s (pid %d)", processName(pid), pid)
}

// nextFree tries the ports above port on the same host until one binds.
func nextFree(addr string, port int) (net.Listener, error) {
	host, _, _ := net.SplitHostPort(addr)
	for p := port + 1; p <= port+100 && p <= 65535; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			return ln, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no free port above %s", addr)
}

func killAndWait(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("pid %d did not exit after SIGTERM", pid)
}

// portHolder returns the pid of the process listening on a TCP port, found
// via /proc, or 0 when it cannot be determined.
func portHolder(port int) (int, error) {
	inodes := map[string]bool{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			f := strings.Fields(line)
			// sl local_address rem_address st ... inode(9)
			if len(f) < 10 || f[3] != "0A" { // 0A = LISTEN
				continue
			}
			_, hexPort, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			p, err := strconv.ParseInt(hexPort, 16, 32)
			if err != nil || int(p) != port {
				continue
			}
			inodes[f[9]] = true
		}
	}
	if len(inodes) == 0 {
		return 0, fmt.Errorf("no listener on port %d in /proc/net", port)
	}
	procs, _ := filepath.Glob("/proc/[0-9]*/fd/*")
	for _, fd := range procs {
		target, err := os.Readlink(fd)
		if err != nil || !strings.HasPrefix(target, "socket:[") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
		if inodes[inode] {
			pid, _ := strconv.Atoi(strings.Split(fd, "/")[2])
			return pid, nil
		}
	}
	return 0, fmt.Errorf("no process owns the listener on port %d", port)
}

func processName(pid int) string {
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(comm))
}
