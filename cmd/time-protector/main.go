package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"time-protector/internal/config"
	"time-protector/internal/dnscheck"
	"time-protector/internal/schedule"
	"time-protector/internal/service"
)

const chromeWarning = "Chrome secure DNS is on, so these sites can still open. Turn off Use secure DNS in Chrome settings, under Privacy and security, then Security."

var errUsage = errors.New("usage")

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return nil
	case "status":
		if len(args) != 1 {
			usage()
			return errUsage
		}
		return runStatus()
	case "lock":
		if len(args) != 1 {
			usage()
			return errUsage
		}
		return runLock()
	case "unlock":
		switch len(args) {
		case 2:
			return runUnlock("", args[1])
		case 3:
			return runUnlock(args[1], args[2])
		default:
			usage()
			return errUsage
		}
	case "schedule":
		if len(args) < 2 {
			usage()
			return errUsage
		}
		return runSchedule(args[1:])
	case "daemon":
		if len(args) != 1 {
			usage()
			return errUsage
		}
		return runDaemon()
	case "write-default-config":
		if len(args) != 2 {
			usage()
			return errUsage
		}
		return writeDefaultConfig(args[1])
	default:
		usage()
		return errUsage
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `time-protector blocks Facebook, Instagram, and LinkedIn during set hours.
WhatsApp and Gmail take fewer questions and do not open the others.

  time-protector status
  time-protector unlock 15m
  time-protector unlock gmail 15m
  time-protector unlock whatsapp 15m
  time-protector lock
  time-protector schedule 08:00-12:00 13:00-18:00

Install the root service with: sudo make install

unlock 15m is 10 questions and opens every blocked site.
unlock gmail 15m and unlock whatsapp 15m are 5 questions and open only WhatsApp and Gmail.
A wrong answer does not count, and the explanation is shown.
Quit and you start over.
The longest break is 30 minutes.
`)
}

func runStatus() error {
	resp, err := call(service.Request{Op: "status"})
	if err != nil {
		warnChrome()
		return err
	}
	if !resp.OK {
		warnChrome()
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Reason)
	warnChrome()
	return nil
}

func runLock() error {
	resp, err := call(service.Request{Op: "lock"})
	if err != nil {
		return err
	}
	if resp.Reason != "" {
		fmt.Println(resp.Reason)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

func runUnlock(target, grant string) error {
	return runSession(service.SocketPath, os.Stdin, os.Stdout, service.Request{Op: "unlock", Target: target, Grant: grant})
}

func runSchedule(windows []string) error {
	return runSession(service.SocketPath, os.Stdin, os.Stdout, service.Request{Op: "schedule", Windows: windows})
}

func runSession(socket string, in io.Reader, out io.Writer, first service.Request) error {
	conn, err := dial(socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	if err := enc.Encode(first); err != nil {
		return err
	}
	var resp service.Response
	if err := dec.Decode(&resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if resp.Capped {
		fmt.Fprintln(out, "30m is the longest break.")
	}
	if resp.Prompt == "" {
		if resp.Reason != "" {
			fmt.Fprintln(out, resp.Reason)
		}
		return nil
	}
	fmt.Fprintf(out, "Answer %d questions. A wrong answer does not count. Quit and you start over.\n", resp.Total)
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "%d/%d\n%s\n", resp.Correct, resp.Total, resp.Prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = enc.Encode(service.Request{Op: "cancel", Session: resp.Session})
				var ack service.Response
				_ = dec.Decode(&ack)
				return nil
			}
			return err
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if len(choice) != 1 || choice[0] < 'a' || choice[0] > 'd' {
			fmt.Fprintln(out, "Type a, b, c, or d.")
			continue
		}
		prev := resp.Correct
		if err := enc.Encode(service.Request{Op: "answer", Session: resp.Session, Choice: choice}); err != nil {
			return err
		}
		resp = service.Response{}
		if err := dec.Decode(&resp); err != nil {
			return err
		}
		if !resp.OK {
			if resp.Reason != "" {
				fmt.Fprintln(out, resp.Reason)
			}
			return errors.New(resp.Error)
		}
		if resp.Explain != "" {
			if resp.Correct == prev && !resp.Done {
				fmt.Fprintln(out, "Not yet.")
			} else {
				fmt.Fprintln(out, "Yes.")
			}
			fmt.Fprintln(out, resp.Explain)
		}
		if resp.Prompt == "" || resp.Done {
			if resp.Reason != "" {
				fmt.Fprintln(out, resp.Reason)
			}
			return nil
		}
	}
}

func runDaemon() error {
	log.SetPrefix("time-protector: ")
	log.SetFlags(0)
	svc := service.New(service.DefaultPaths())
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return svc.Run(ctx)
}

func writeDefaultConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return config.Save(path, schedule.Default())
}

func call(req service.Request) (service.Response, error) {
	conn, err := dial(service.SocketPath)
	if err != nil {
		return service.Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return service.Response{}, err
	}
	var resp service.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return service.Response{}, err
	}
	return resp, nil
}

func dial(socket string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("the blocker is not running. Install it with sudo make install")
	}
	return conn, nil
}

func warnChrome() {
	dir, err := dnscheck.DefaultChromeDir()
	if err != nil {
		return
	}
	on, err := dnscheck.AnyProfileSecureDNS(dir)
	if err != nil || !on {
		return
	}
	fmt.Println(chromeWarning)
}
