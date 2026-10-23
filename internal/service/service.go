// Package service is the root daemon. It owns the hosts file, the lease, and the answer key.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	mrand "math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"time-protector/internal/blocklist"
	"time-protector/internal/config"
	"time-protector/internal/hosts"
	"time-protector/internal/lease"
	"time-protector/internal/quiz"
	"time-protector/internal/schedule"
)

const (
	// SocketPath is the daemon socket. Mode 0666 so the user can connect.
	SocketPath = "/var/run/time-protector.sock"
	// HostsPath is the system hosts file.
	HostsPath = "/etc/hosts"

	reconcileEvery = 2 * time.Second
)

// Paths are the files the daemon uses.
type Paths struct {
	Config   string
	State    string
	Question string
	Secret   string
	Socket   string
	Hosts    string
	Backup   string
}

// DefaultPaths are the installed locations.
func DefaultPaths() Paths {
	return Paths{
		Config:   "/usr/local/etc/time-protector/config.yaml",
		State:    "/usr/local/var/time-protector/state.json",
		Question: "/usr/local/var/time-protector/question",
		Secret:   "/usr/local/var/time-protector/secret",
		Socket:   SocketPath,
		Hosts:    HostsPath,
		Backup:   "/usr/local/var/time-protector/hosts.backup",
	}
}

// Request is one command from the user.
type Request struct {
	Op      string   `json:"op"`
	Grant   string   `json:"grant,omitempty"`
	Target  string   `json:"target,omitempty"`
	Session string   `json:"session,omitempty"`
	Choice  string   `json:"choice,omitempty"`
	Windows []string `json:"windows,omitempty"`
}

// Response is the daemon's reply. It never includes the answer.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
	Session string `json:"session,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Explain string `json:"explain,omitempty"`
	Correct int    `json:"correct,omitempty"`
	Total   int    `json:"total,omitempty"`
	Done    bool   `json:"done"`
	Capped  bool   `json:"capped,omitempty"`
}

type quizSession struct {
	id       string
	kind     string
	scope    string
	need     int
	grant    time.Duration
	capped   bool
	windows  []schedule.Window
	progress lease.Session
	asked    map[string]bool
	prompt   string
	letter   string
	why      string
}

type diskState struct {
	LeaseUntil  time.Time `json:"leaseUntil,omitempty"`
	SocialUntil time.Time `json:"socialUntil,omitempty"`
	GmailUntil  time.Time `json:"gmailUntil,omitempty"`
}

// Service is the long-running blocker.
type Service struct {
	paths Paths
	// Clock overrides the clock in tests. The installed daemon leaves it nil.
	Clock func() time.Time

	ready chan struct{}
	ln    net.Listener

	mu          sync.Mutex
	rng         *mrand.Rand
	session     *quizSession
	socialUntil time.Time
	gmailUntil  time.Time
	cached      *schedule.Schedule
	configErr   string
	hostsErr    string
}

// New builds a service. Call Run to listen.
func New(paths Paths) *Service {
	return &Service{
		paths: paths,
		ready: make(chan struct{}),
	}
}

// Run listens until ctx is cancelled. It refreshes the hosts file every few seconds.
func (s *Service) Run(ctx context.Context) error {
	s.secureInstalledDirs()
	if err := os.MkdirAll(filepath.Dir(s.paths.State), 0o755); err != nil {
		return err
	}
	if err := s.loadState(); err != nil {
		log.Printf("state: %v", err)
	}
	if err := s.listen(); err != nil {
		return err
	}
	defer func() {
		s.ln.Close()
		_ = os.Remove(s.paths.Socket)
	}()
	_ = s.reconcile()
	go s.acceptLoop()
	close(s.ready)
	log.Printf("daemon started")
	ticker := time.NewTicker(reconcileEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = s.reconcile()
		}
	}
}

func (s *Service) secureInstalledDirs() {
	secureDir(filepath.Dir(s.paths.State))
	secureDir(filepath.Dir(s.paths.Config))
}

func secureDir(dir string) {
	if !strings.HasPrefix(dir, "/usr/local/") {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if err := os.Chown(dir, 0, 0); err != nil {
		return
	}
	_ = os.Chmod(dir, 0o755)
}

func (s *Service) listen() error {
	if err := os.MkdirAll(filepath.Dir(s.paths.Socket), 0o755); err != nil {
		return err
	}
	_ = os.Remove(s.paths.Socket)
	ln, err := net.Listen("unix", s.paths.Socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.paths.Socket, 0o666); err != nil {
		ln.Close()
		return err
	}
	s.ln = ln
	return nil
}

func (s *Service) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.serveConn(conn)
	}
}

func (s *Service) serveConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	var sessionID string
	defer func() {
		s.mu.Lock()
		s.abandon(sessionID)
		s.mu.Unlock()
	}()
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		resp := s.handle(&sessionID, req)
		if err := enc.Encode(&resp); err != nil {
			return
		}
	}
}

func (s *Service) handle(sessionID *string, req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch req.Op {
	case "status":
		return s.statusResp()
	case "unlock":
		resp := s.begin("unlock", req)
		if resp.Session != "" {
			*sessionID = resp.Session
		}
		return resp
	case "schedule":
		resp := s.begin("schedule", req)
		if resp.Session != "" {
			*sessionID = resp.Session
		}
		return resp
	case "answer":
		return s.answer(req)
	case "cancel":
		s.abandon(req.Session)
		if req.Session == *sessionID {
			*sessionID = ""
		}
		return Response{OK: true}
	case "lock":
		return s.lock()
	default:
		return Response{OK: false, Error: "unknown command"}
	}
}

func (s *Service) begin(kind string, req Request) Response {
	sess := &quizSession{kind: kind, need: lease.SocialLength, asked: map[string]bool{}}
	switch kind {
	case "unlock":
		grant, capped, err := lease.ParseGrant(req.Grant)
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		scope, need, err := unlockScope(req.Target)
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		now := s.now()
		inWindow := s.schedule().Blocks(now)
		socialOpen := lease.Active(s.socialUntil, now)
		if !inWindow || socialOpen || (scope == "short" && lease.Active(s.gmailUntil, now)) {
			return s.statusResp()
		}
		sess.scope = scope
		sess.need = need
		sess.grant = grant
		sess.capped = capped
	case "schedule":
		if len(req.Windows) == 0 {
			return Response{OK: false, Error: "add at least one window, like 08:00-12:00"}
		}
		for _, raw := range req.Windows {
			window, err := schedule.ParseWindow(raw)
			if err != nil {
				return Response{OK: false, Error: err.Error()}
			}
			sess.windows = append(sess.windows, window)
		}
	default:
		return Response{OK: false, Error: "unknown command"}
	}
	id, err := newID()
	if err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	sess.id = id
	if err := s.deal(sess); err != nil {
		if s.session == nil {
			s.clearQuizFiles()
		}
		return Response{OK: false, Error: err.Error()}
	}
	s.session = sess
	return Response{
		OK:      true,
		Blocked: s.blockedNow(),
		Session: sess.id,
		Prompt:  sess.prompt,
		Total:   sess.need,
		Capped:  sess.capped,
	}
}

func unlockScope(target string) (string, int, error) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "", "all", "social":
		return "social", lease.SocialLength, nil
	case "gmail", "whatsapp":
		return "short", lease.ShortLength, nil
	default:
		return "", 0, fmt.Errorf("use unlock 15m, unlock gmail 15m, or unlock whatsapp 15m")
	}
}

func (s *Service) answer(req Request) Response {
	sess := s.session
	if sess == nil || sess.id != req.Session {
		return Response{OK: false, Error: "that quiz is no longer active"}
	}
	if sess.progress.Correct >= sess.need {
		return s.finish(sess)
	}
	// Grade from the root-only file so the client cannot mark the quiz done.
	why := sess.why
	right, err := s.grade(req.Session, req.Choice)
	if err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	if right && sess.progress.Accept(true, sess.need) {
		resp := s.finish(sess)
		resp.Explain = why
		return resp
	}
	if err := s.deal(sess); err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	return Response{
		OK:      true,
		Blocked: s.blockedNow(),
		Session: sess.id,
		Prompt:  sess.prompt,
		Explain: why,
		Correct: sess.progress.Correct,
		Total:   sess.need,
	}
}

func (s *Service) finish(sess *quizSession) Response {
	switch sess.kind {
	case "unlock":
		previousSocial := s.socialUntil
		previousGmail := s.gmailUntil
		until := s.now().Add(sess.grant)
		if sess.scope == "short" {
			s.gmailUntil = until
		} else {
			s.socialUntil = until
			s.gmailUntil = until
		}
		if err := s.saveState(); err != nil {
			s.socialUntil = previousSocial
			s.gmailUntil = previousGmail
			return Response{OK: false, Error: err.Error()}
		}
	case "schedule":
		next := schedule.Schedule{Windows: append([]schedule.Window(nil), sess.windows...)}
		if err := config.Save(s.paths.Config, next); err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		s.cached = &next
	default:
		return Response{OK: false, Error: "unknown command"}
	}
	s.session = nil
	s.clearQuizFiles()
	if err := s.reconcileLocked(); err != nil {
		resp := s.statusResp()
		resp.OK = false
		resp.Error = err.Error()
		resp.Done = true
		resp.Correct = sess.progress.Correct
		resp.Total = sess.need
		return resp
	}
	resp := s.statusResp()
	resp.Done = true
	resp.Correct = sess.progress.Correct
	resp.Total = sess.need
	return resp
}

func (s *Service) lock() Response {
	s.socialUntil = time.Time{}
	s.gmailUntil = time.Time{}
	if err := s.saveState(); err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	if err := s.reconcileLocked(); err != nil {
		resp := s.statusResp()
		resp.OK = false
		resp.Error = err.Error()
		return resp
	}
	return s.statusResp()
}

func (s *Service) abandon(id string) {
	if id == "" || s.session == nil || s.session.id != id {
		return
	}
	s.session.progress.Reset()
	s.session = nil
	s.clearQuizFiles()
}

func (s *Service) deal(sess *quizSession) error {
	if sess.asked == nil {
		sess.asked = map[string]bool{}
	}
	item := quiz.Draw(s.random(), sess.asked)
	sess.asked[item.ID] = true
	secret := sess.id + "\n" + item.Letter() + "\n"
	if err := writeAtomic(s.paths.Secret, []byte(secret), 0o600); err != nil {
		return err
	}
	if err := writeAtomic(s.paths.Question, []byte(item.Prompt()+"\n"), 0o644); err != nil {
		if sess.prompt != "" {
			prev := sess.id + "\n" + sess.letter + "\n"
			_ = writeAtomic(s.paths.Secret, []byte(prev), 0o600)
		}
		return err
	}
	sess.prompt = item.Prompt()
	sess.letter = item.Letter()
	sess.why = item.Explain()
	return nil
}

func (s *Service) grade(sessionID, choice string) (bool, error) {
	data, err := os.ReadFile(s.paths.Secret)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("that quiz is no longer active")
		}
		return false, err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || lines[0] != sessionID {
		return false, fmt.Errorf("that quiz is no longer active")
	}
	return strings.ToLower(strings.TrimSpace(choice)) == lines[1], nil
}

func (s *Service) clearQuizFiles() {
	_ = os.Remove(s.paths.Question)
	_ = os.Remove(s.paths.Secret)
}

func (s *Service) random() *mrand.Rand {
	if s.rng == nil {
		s.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	}
	return s.rng
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Service) blockedNow() bool {
	return len(s.blockedDomains(s.now())) > 0
}

func (s *Service) blockedDomains(now time.Time) []string {
	if !s.schedule().Blocks(now) {
		return nil
	}
	var out []string
	if !lease.Active(s.socialUntil, now) {
		out = append(out, blocklist.Social()...)
	}
	if !lease.Active(s.socialUntil, now) && !lease.Active(s.gmailUntil, now) {
		out = append(out, blocklist.Short()...)
	}
	return out
}

func (s *Service) schedule() schedule.Schedule {
	loaded, err := config.Load(s.paths.Config)
	if err != nil {
		if !os.IsNotExist(err) && s.configErr != err.Error() {
			log.Printf("config: %v", err)
			s.configErr = err.Error()
		}
		if s.cached != nil {
			return *s.cached
		}
		return schedule.Default()
	}
	s.configErr = ""
	if len(loaded.Windows) == 0 {
		return schedule.Default()
	}
	s.cached = &loaded
	return loaded
}

func (s *Service) statusResp() Response {
	now := s.now()
	sched := s.schedule()
	inWindow := sched.Blocks(now)
	socialOpen := lease.Active(s.socialUntil, now)
	shortOpen := socialOpen || lease.Active(s.gmailUntil, now)
	blocked := inWindow && !socialOpen
	var reason string
	switch {
	case inWindow && !socialOpen && !shortOpen:
		reason = fmt.Sprintf("Blocked until %s.", human(now, sched.NextChange(now)))
	case inWindow && socialOpen:
		reason = fmt.Sprintf("Open until %s.", human(now, s.socialUntil))
	case inWindow && shortOpen:
		reason = fmt.Sprintf("WhatsApp and Gmail are open until %s. Facebook, Instagram, and LinkedIn stay blocked until %s.", human(now, s.gmailUntil), human(now, sched.NextChange(now)))
		blocked = true
	default:
		reason = fmt.Sprintf("Open until %s.", human(now, sched.NextChange(now)))
		blocked = false
	}
	if s.hostsErr != "" {
		reason += " Hosts file was not updated: " + s.hostsErr
	}
	return Response{OK: true, Blocked: blocked, Reason: reason}
}

func human(now, until time.Time) string {
	if until.IsZero() {
		return "later"
	}
	clock := until.Format("15:04")
	if until.Second() != 0 || until.Nanosecond() != 0 {
		clock = until.Format("15:04:05")
	}
	if until.Year() == now.Year() && until.YearDay() == now.YearDay() {
		return clock
	}
	nextDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	if until.Year() == nextDay.Year() && until.YearDay() == nextDay.YearDay() {
		return clock + " tomorrow"
	}
	return until.Format("Mon 15:04")
}

func (s *Service) reconcile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileLocked()
}

func (s *Service) reconcileLocked() error {
	now := s.now()
	expired := false
	if !s.socialUntil.IsZero() && !lease.Active(s.socialUntil, now) {
		s.socialUntil = time.Time{}
		expired = true
	}
	if !s.gmailUntil.IsZero() && !lease.Active(s.gmailUntil, now) {
		s.gmailUntil = time.Time{}
		expired = true
	}
	if expired {
		if err := s.saveState(); err != nil {
			s.noteHostsErr(err)
			return err
		}
	}
	current, err := os.ReadFile(s.paths.Hosts)
	if err != nil {
		s.noteHostsErr(err)
		return err
	}
	domains := s.blockedDomains(now)
	next, err := hosts.Apply(string(current), domains, len(domains) > 0)
	if err != nil {
		s.noteHostsErr(err)
		return err
	}
	s.hostsErr = ""
	if next == string(current) {
		return nil
	}
	if err := s.backupHosts(); err != nil {
		s.noteHostsErr(err)
		return err
	}
	if err := writeAtomic(s.paths.Hosts, []byte(next), 0o644); err != nil {
		s.noteHostsErr(err)
		return err
	}
	if s.paths.Hosts == HostsPath {
		flushDNS()
	}
	if s.blockedNow() {
		log.Printf("sites blocked")
	} else {
		log.Printf("sites open")
	}
	return nil
}

func (s *Service) noteHostsErr(err error) {
	if err == nil {
		s.hostsErr = ""
		return
	}
	if s.hostsErr != err.Error() {
		log.Printf("hosts: %v", err)
	}
	s.hostsErr = err.Error()
}

func (s *Service) backupHosts() error {
	if s.paths.Backup == "" {
		return nil
	}
	if _, err := os.Stat(s.paths.Backup); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(s.paths.Hosts)
	if err != nil {
		return err
	}
	return writeAtomic(s.paths.Backup, data, 0o600)
}

func (s *Service) loadState() error {
	data, err := os.ReadFile(s.paths.State)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var disk diskState
	if err := json.Unmarshal(data, &disk); err != nil {
		return err
	}
	s.mu.Lock()
	s.socialUntil = disk.SocialUntil
	s.gmailUntil = disk.GmailUntil
	if s.socialUntil.IsZero() && s.gmailUntil.IsZero() && !disk.LeaseUntil.IsZero() {
		s.socialUntil = disk.LeaseUntil
		s.gmailUntil = disk.LeaseUntil
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) saveState() error {
	data, err := json.Marshal(diskState{SocialUntil: s.socialUntil, GmailUntil: s.gmailUntil})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(s.paths.State, data, 0o600)
}

func flushDNS() {
	_ = exec.Command("dscacheutil", "-flushcache").Run()
	_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
}

func newID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
