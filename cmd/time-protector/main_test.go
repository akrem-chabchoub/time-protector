package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"time-protector/internal/service"
)

func TestWriteDefaultConfigOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := writeDefaultConfig(path); err != nil {
		t.Fatal(err)
	}
	if err := writeDefaultConfig(path); err == nil {
		t.Fatal("expected the second write to fail")
	}
}

func TestUnlockLoop(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join("/tmp", fmt.Sprintf("tp-cli-%d.sock", os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(sock) })
	paths := service.Paths{
		Config:   filepath.Join(dir, "config.yaml"),
		State:    filepath.Join(dir, "state.json"),
		Question: filepath.Join(dir, "question"),
		Secret:   filepath.Join(dir, "secret"),
		Socket:   sock,
		Hosts:    hostsPath,
		Backup:   filepath.Join(dir, "hosts.backup"),
	}
	when := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	svc := service.New(paths)
	svc.Clock = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return when
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svc.Run(ctx) }()
	waitForSocket(t, sock)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- runSession(sock, inR, outW, service.Request{Op: "unlock", Grant: "15m"})
		_ = outW.Close()
	}()
	timer := time.AfterFunc(10*time.Second, func() {
		_ = inR.Close()
		_ = outR.Close()
	})
	defer timer.Stop()

	var sawNope, sawType int
	var reason string
	reader := bufio.NewReader(outR)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "Not yet.":
			sawNope++
		case line == "Type a, b, c, or d.":
			sawType++
		case line == "Yes." || strings.HasPrefix(line, "Answer ") || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "  "):
			if strings.HasPrefix(line, "  d) ") {
				switch {
				case sawType == 0:
					fmt.Fprintln(inW, "nope")
				case sawNope == 0:
					fmt.Fprintln(inW, otherLetter(t, paths.Secret))
				default:
					fmt.Fprintln(inW, secretLetter(t, paths.Secret))
				}
			}
		default:
			reason = line
		}
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if sawType != 1 || sawNope != 1 {
		t.Fatalf("type=%d nope=%d reason=%q", sawType, sawNope, reason)
	}
	if reason != "Open until 10:15." {
		t.Fatalf("reason %q", reason)
	}
}

func waitForSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func secretLetter(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || len(lines[1]) != 1 {
		t.Fatalf("secret %q", data)
	}
	return lines[1]
}

func otherLetter(t *testing.T, path string) string {
	t.Helper()
	if secretLetter(t, path) == "a" {
		return "b"
	}
	return "a"
}
