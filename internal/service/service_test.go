package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"time-protector/internal/config"
	"time-protector/internal/hosts"
	"time-protector/internal/lease"
	"time-protector/internal/schedule"
)

var sockN atomic.Int64

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 22, hour, minute, 0, 0, time.UTC)
}

type lockedClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *lockedClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *lockedClock) set(t time.Time) {
	c.mu.Lock()
	c.at = t
	c.mu.Unlock()
}

type env struct {
	t      *testing.T
	svc    *Service
	paths  Paths
	clock  *lockedClock
	cancel context.CancelFunc
	done   chan struct{}
}

func startAt(t *testing.T, when time.Time) *env {
	t.Helper()
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	original := "127.0.0.1 localhost\n# keep me\n"
	if err := os.WriteFile(hostsPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	sock := fmt.Sprintf("/tmp/tp-%d-%d.sock", os.Getpid(), sockN.Add(1))
	paths := Paths{
		Config:   filepath.Join(dir, "config.yaml"),
		State:    filepath.Join(dir, "state.json"),
		Question: filepath.Join(dir, "question"),
		Secret:   filepath.Join(dir, "secret"),
		Socket:   sock,
		Hosts:    hostsPath,
		Backup:   filepath.Join(dir, "hosts.backup"),
	}
	if err := config.Save(paths.Config, schedule.Default()); err != nil {
		t.Fatal(err)
	}
	clock := &lockedClock{at: when}
	svc := New(paths)
	svc.Clock = clock.now
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = svc.Run(ctx)
		close(done)
	}()
	out := &env{t: t, svc: svc, paths: paths, clock: clock, cancel: cancel, done: done}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		_ = os.Remove(sock)
	})
	select {
	case <-svc.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not start")
	}
	return out
}

func (e *env) readHosts() string {
	e.t.Helper()
	data, err := os.ReadFile(e.paths.Hosts)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

type rpcClient struct {
	t    *testing.T
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
}

func (e *env) dial() *rpcClient {
	e.t.Helper()
	conn, err := net.Dial("unix", e.paths.Socket)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = conn.Close() })
	return &rpcClient{t: e.t, conn: conn, enc: json.NewEncoder(conn), dec: json.NewDecoder(conn)}
}

func (c *rpcClient) call(req Request) Response {
	c.t.Helper()
	if err := c.enc.Encode(req); err != nil {
		c.t.Fatal(err)
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func (e *env) letter(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(e.paths.Secret)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || len(lines[1]) != 1 {
		t.Fatalf("secret %q", data)
	}
	return lines[1]
}

func answerUntilDone(t *testing.T, e *env, c *rpcClient, resp Response) Response {
	t.Helper()
	for resp.Prompt != "" {
		if !resp.OK {
			t.Fatal(resp.Error)
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"answer"`) {
			t.Fatalf("response leaked an answer field: %s", raw)
		}
		resp = c.call(Request{Op: "answer", Session: resp.Session, Choice: e.letter(t)})
	}
	if !resp.OK {
		t.Fatalf("%+v", resp)
	}
	return resp
}

func TestSecureDirLeavesOtherFolders(t *testing.T) {
	dir := t.TempDir()
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	secureDir(dir)
	after, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode() != after.Mode() {
		t.Fatalf("mode changed from %s to %s", before.Mode(), after.Mode())
	}
}

func TestHoursAndHosts(t *testing.T) {
	original := "127.0.0.1 localhost\n# keep me\n"
	blocked := startAt(t, at(10, 0))
	body := blocked.readHosts()
	if strings.Count(body, "127.0.0.1 localhost\n") != 1 || strings.Count(body, "# keep me\n") != 1 {
		t.Fatalf("unrelated lines changed:\n%s", body)
	}
	for _, domain := range []string{"facebook.com", "instagram.com", "web.whatsapp.com", "linkedin.com", "mail.google.com"} {
		if !strings.Contains(body, "127.0.0.1 "+domain+"\n") || !strings.Contains(body, "::1 "+domain+"\n") {
			t.Fatalf("missing %s:\n%s", domain, body)
		}
	}
	if strings.Contains(body, "accounts.google.com") || strings.Contains(body, "imap.gmail.com") {
		t.Fatalf("blocked too much:\n%s", body)
	}
	backup, err := os.ReadFile(blocked.paths.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != original {
		t.Fatalf("backup %q", backup)
	}
	info, err := os.Stat(blocked.paths.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode %s", info.Mode().Perm())
	}
	status := blocked.dial().call(Request{Op: "status"})
	if !status.Blocked || status.Reason != "Blocked until 12:00." {
		t.Fatalf("%+v", status)
	}

	lunch := startAt(t, at(12, 30))
	if lunch.readHosts() != original {
		t.Fatalf("lunch changed hosts:\n%s", lunch.readHosts())
	}
	lunchStatus := lunch.dial().call(Request{Op: "status"})
	if lunchStatus.Blocked || lunchStatus.Reason != "Open until 13:00." {
		t.Fatalf("%+v", lunchStatus)
	}

	night := startAt(t, at(19, 0))
	nightStatus := night.dial().call(Request{Op: "status"})
	if nightStatus.Blocked || nightStatus.Reason != "Open until 08:00 tomorrow." {
		t.Fatalf("%+v", nightStatus)
	}
}

func TestQuizWrongAnswerCapAndExpiry(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	resp := c.call(Request{Op: "unlock", Grant: "15m"})
	if !resp.OK || resp.Capped || resp.Prompt == "" || resp.Correct != 0 || resp.Total != lease.SocialLength || resp.Explain != "" {
		t.Fatalf("%+v", resp)
	}
	info, err := os.Stat(e.paths.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret mode %s", info.Mode().Perm())
	}
	question, err := os.ReadFile(e.paths.Question)
	if err != nil {
		t.Fatal(err)
	}
	if string(question) != resp.Prompt+"\n" {
		t.Fatalf("question file %q", question)
	}
	qinfo, err := os.Stat(e.paths.Question)
	if err != nil {
		t.Fatal(err)
	}
	if qinfo.Mode().Perm() != 0o644 {
		t.Fatalf("question mode %s", qinfo.Mode().Perm())
	}

	wrong := c.call(Request{Op: "answer", Session: resp.Session, Choice: "z"})
	if !wrong.OK || wrong.Correct != 0 || wrong.Done || wrong.Prompt == "" || wrong.Explain == "" {
		t.Fatalf("wrong answer counted: %+v", wrong)
	}
	resp = wrong
	for i := 0; i < lease.SocialLength-1; i++ {
		resp = c.call(Request{Op: "answer", Session: resp.Session, Choice: e.letter(t)})
		if !resp.OK || resp.Done {
			t.Fatalf("step %d: %+v", i, resp)
		}
	}
	if resp.Correct != lease.SocialLength-1 {
		t.Fatalf("correct %d", resp.Correct)
	}
	if !strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("block dropped before the last correct answer")
	}
	resp = c.call(Request{Op: "answer", Session: resp.Session, Choice: e.letter(t)})
	if !resp.Done || resp.Correct != lease.SocialLength || resp.Reason != "Open until 10:15." {
		t.Fatalf("%+v", resp)
	}
	if strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("sites stayed blocked after the quiz")
	}
	if strings.Count(e.readHosts(), "127.0.0.1 localhost\n") != 1 {
		t.Fatalf("localhost changed:\n%s", e.readHosts())
	}

	again := c.call(Request{Op: "unlock", Grant: "15m"})
	if again.Prompt != "" || again.Reason != "Open until 10:15." {
		t.Fatalf("second unlock started a quiz: %+v", again)
	}

	e.clock.set(at(10, 15))
	if err := e.svc.reconcile(); err != nil {
		t.Fatal(err)
	}
	after := e.dial().call(Request{Op: "status"})
	if !after.Blocked || after.Reason != "Blocked until 12:00." {
		t.Fatalf("%+v", after)
	}
	if !strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("block did not return when the lease ended")
	}
}

func TestGrantCap(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	resp := c.call(Request{Op: "unlock", Grant: "2h"})
	if !resp.Capped {
		t.Fatal("expected the 30 minute cap")
	}
	done := answerUntilDone(t, e, c, resp)
	if done.Reason != "Open until 10:30." {
		t.Fatalf("%+v", done)
	}
	data, err := os.ReadFile(e.paths.State)
	if err != nil {
		t.Fatal(err)
	}
	var disk diskState
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if !disk.SocialUntil.Equal(at(10, 30)) || !disk.GmailUntil.Equal(at(10, 30)) {
		t.Fatalf("lease social %s gmail %s", disk.SocialUntil, disk.GmailUntil)
	}
}

func TestLockEndsLease(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	done := answerUntilDone(t, e, c, c.call(Request{Op: "unlock", Grant: "15m"}))
	if done.Reason != "Open until 10:15." {
		t.Fatalf("%+v", done)
	}
	locked := c.call(Request{Op: "lock"})
	if !locked.Blocked || locked.Reason != "Blocked until 12:00." {
		t.Fatalf("%+v", locked)
	}
	if !strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("lock did not restore the block")
	}
}

func TestCancelResets(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	resp := c.call(Request{Op: "unlock", Grant: "15m"})
	resp = c.call(Request{Op: "answer", Session: resp.Session, Choice: e.letter(t)})
	if resp.Correct != 1 {
		t.Fatalf("%+v", resp)
	}
	ack := c.call(Request{Op: "cancel", Session: resp.Session})
	if !ack.OK {
		t.Fatal(ack.Error)
	}
	if _, err := os.Stat(e.paths.Secret); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancel left the answer key in place")
	}
	status := c.call(Request{Op: "status"})
	if !status.Blocked {
		t.Fatal("cancel opened the sites")
	}
	again := c.call(Request{Op: "unlock", Grant: "15m"})
	if again.Correct != 0 || again.Prompt == "" {
		t.Fatalf("quit did not reset: %+v", again)
	}
}

func TestDisconnectResets(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	resp := c.call(Request{Op: "unlock", Grant: "15m"})
	resp = c.call(Request{Op: "answer", Session: resp.Session, Choice: e.letter(t)})
	if resp.Correct != 1 {
		t.Fatalf("%+v", resp)
	}
	if err := c.conn.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := os.Stat(e.paths.Secret)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnect did not reset the quiz")
		}
		time.Sleep(10 * time.Millisecond)
	}
	again := e.dial().call(Request{Op: "unlock", Grant: "15m"})
	if again.Correct != 0 || again.Prompt == "" {
		t.Fatalf("quit did not reset: %+v", again)
	}
}

func TestScheduleWaitsForQuiz(t *testing.T) {
	e := startAt(t, at(10, 0))
	before, err := os.ReadFile(e.paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	c := e.dial()
	resp := c.call(Request{Op: "schedule", Windows: []string{"14:00-15:00"}})
	if resp.Prompt == "" {
		t.Fatalf("%+v", resp)
	}
	during, err := os.ReadFile(e.paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(during) != string(before) {
		t.Fatal("schedule changed before the quiz finished")
	}
	done := answerUntilDone(t, e, c, resp)
	if done.Reason != "Open until 14:00." {
		t.Fatalf("%+v", done)
	}
	loaded, err := config.Load(e.paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Windows) != 1 || loaded.Windows[0].Start != 14*60 || loaded.Windows[0].End != 15*60 {
		t.Fatalf("windows %+v", loaded.Windows)
	}
	if strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("10:00 should be open after the hours change")
	}
	rejected := c.call(Request{Op: "schedule"})
	if rejected.OK {
		t.Fatal("empty schedule was accepted")
	}
}

func TestTamperAndUnknownCommand(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	resp := c.call(Request{Op: "unlock", Grant: "15m"})
	data, err := os.ReadFile(e.paths.Secret)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("secret %q", data)
	}
	if err := os.WriteFile(e.paths.Secret, []byte(lines[0]+"\nz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	graded := c.call(Request{Op: "answer", Session: resp.Session, Choice: lines[1]})
	if graded.Correct != 0 || graded.Done {
		t.Fatalf("tampered answer counted: %+v", graded)
	}
	unknown := c.call(Request{Op: "done"})
	if unknown.OK || unknown.Error != "unknown command" {
		t.Fatalf("%+v", unknown)
	}
	if !strings.Contains(e.readHosts(), hosts.Begin) {
		t.Fatal("unknown command removed the block")
	}
}

func TestBrokenHostsIsLeftAlone(t *testing.T) {
	e := startAt(t, at(10, 0))
	broken := hosts.Begin + "\n127.0.0.1 mail.google.com\n"
	if err := os.WriteFile(e.paths.Hosts, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.reconcile(); err == nil {
		t.Fatal("expected an error for a broken hosts block")
	}
	got, err := os.ReadFile(e.paths.Hosts)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != broken {
		t.Fatalf("broken hosts was rewritten:\n%s", got)
	}
	status := e.dial().call(Request{Op: "status"})
	if !strings.Contains(status.Reason, "missing the end marker") {
		t.Fatalf("%+v", status)
	}
}

func TestOutsideHoursSkipsQuiz(t *testing.T) {
	e := startAt(t, at(12, 30))
	resp := e.dial().call(Request{Op: "unlock", Grant: "15m"})
	if resp.Prompt != "" || resp.Blocked || resp.Reason != "Open until 13:00." {
		t.Fatalf("%+v", resp)
	}
	if _, err := os.Stat(e.paths.Secret); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("quiz started outside the block")
	}
}

func TestLeaseSurvivesRestart(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	done := answerUntilDone(t, e, c, c.call(Request{Op: "unlock", Grant: "15m"}))
	if done.Reason != "Open until 10:15." {
		t.Fatalf("%+v", done)
	}
	_ = c.conn.Close()
	e.cancel()
	select {
	case <-e.done:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}

	svc := New(e.paths)
	svc.Clock = e.clock.now
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		_ = svc.Run(ctx)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
		}
	})
	select {
	case <-svc.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not restart")
	}
	status := dialPaths(t, e.paths.Socket).call(Request{Op: "status"})
	if status.Blocked || status.Reason != "Open until 10:15." {
		t.Fatalf("%+v", status)
	}
}

func TestGmailUnlockLeavesTheOthersBlocked(t *testing.T) {
	e := startAt(t, at(10, 0))
	c := e.dial()
	started := c.call(Request{Op: "unlock", Target: "gmail", Grant: "15m"})
	if started.Total != lease.ShortLength || started.Session == "" {
		t.Fatalf("%+v", started)
	}
	if ack := c.call(Request{Op: "cancel", Session: started.Session}); !ack.OK {
		t.Fatalf("cancel %+v", ack)
	}
	resp := c.call(Request{Op: "unlock", Target: "whatsapp", Grant: "15m"})
	if resp.Total != lease.ShortLength || resp.Prompt == "" || resp.Explain != "" {
		t.Fatalf("%+v", resp)
	}
	done := answerUntilDone(t, e, c, resp)
	want := "WhatsApp and Gmail are open until 10:15. Facebook, Instagram, and LinkedIn stay blocked until 12:00."
	if !done.Done || done.Reason != want {
		t.Fatalf("%+v", done)
	}
	body := e.readHosts()
	for _, domain := range []string{"gmail.com", "mail.google.com", "web.whatsapp.com", "wa.me"} {
		if strings.Contains(body, "127.0.0.1 "+domain+"\n") {
			t.Fatalf("%s still blocked:\n%s", domain, body)
		}
	}
	for _, domain := range []string{"facebook.com", "instagram.com", "linkedin.com"} {
		if !strings.Contains(body, "127.0.0.1 "+domain+"\n") {
			t.Fatalf("%s opened:\n%s", domain, body)
		}
	}
	again := c.call(Request{Op: "unlock", Grant: "15m"})
	if again.Prompt == "" || again.Total != lease.SocialLength {
		t.Fatalf("social quiz did not start: %+v", again)
	}
}

func dialPaths(t *testing.T, socket string) *rpcClient {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &rpcClient{t: t, conn: conn, enc: json.NewEncoder(conn), dec: json.NewDecoder(conn)}
}
