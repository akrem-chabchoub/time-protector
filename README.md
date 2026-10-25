# time-protector

time-protector is a Mac helper that closes a few distracting sites during work hours. Opening one means answering a short multiple-choice quiz. It is friction for the normal path. It is not a lock the admin of the Mac cannot remove.

The quiz is Kubernetes, Go, and problem solving. A wrong answer shows a one-line explanation and does not count. Quit halfway and the next try starts at zero.

## What it closes

| Sites | Command | Correct answers | What opens |
| --- | --- | --- | --- |
| WhatsApp and Gmail | `time-protector unlock whatsapp 15m` or `time-protector unlock gmail 15m` | 5 | Those two. Facebook, Instagram, and LinkedIn stay closed |
| Facebook, Instagram, and LinkedIn | `time-protector unlock 15m` | 10 | Every blocked site, including WhatsApp and Gmail |

`15m` is the break you are asking for. The longest break is 30 minutes, so `unlock 2h` still opens the sites for 30 minutes. `time-protector lock` closes the break early.

The default hours are 08:00–12:00 and 13:00–18:00, every day, in the Mac's local time. Lunch (12:00–13:00) and the evening are open. 12:00 and 18:00 themselves are open.

Gmail here means the website (`gmail.com` and `mail.google.com`). Google sign-in stays usable.

## Install

You need macOS and Go 1.24. From this folder, as an admin:

```bash
sudo make install
```

That builds the program, copies it to `/usr/local/bin/time-protector`, saves a backup of `/etc/hosts` the first time, and starts a root helper named `com.akrem.time-protector`. The helper starts again after a reboot.

Check it:

```bash
time-protector status
```

During the morning window that prints `Blocked until 12:00.`

Chrome can skip this kind of block. In Chrome, open Settings, Privacy and security, Security, and turn off **Use secure DNS**. `time-protector status` warns when that setting is not explicitly off.

## Daily use

```bash
time-protector status
time-protector unlock 15m
time-protector unlock gmail 15m
time-protector unlock whatsapp 15m
time-protector lock
time-protector schedule 08:00-12:00 13:00-18:00
```

Changing the hours also takes the 10-question quiz. The hours are stored in `/usr/local/etc/time-protector/config.yaml`. An empty or broken windows list falls back to the default hours, so a bad edit does not silently turn the block off.

The site list lives in `internal/blocklist/blocklist.go`. Changing it means editing that file, then running `sudo make install` again. There is no per-site command yet: WhatsApp and Gmail open together, and Facebook, Instagram, and LinkedIn open together.

## How it works

A root helper wakes up every couple of seconds.

During work hours it writes the blocked names at the top of `/etc/hosts`, between `# BEGIN time-protector` and `# END time-protector`, and points them at this Mac. The first matching line in that file wins, which is why the block stays at the top. Lines outside the markers are left alone. The original file is kept once, at `/usr/local/var/time-protector/hosts.backup`.

The quiz answer for the current question is a root-only file. The program you type into does not get to mark itself done. When the quiz finishes, the helper records how long the break lasts and removes the matching names from `/etc/hosts`.

## What this does not stop

- You own the Mac. `sudo` can stop the helper.
- The questions and the right answers are in `internal/quiz/bank.go`. The running quiz hides the answer until you reply, and you can also open that file.
- Chrome with secure DNS on does not read `/etc/hosts` for those lookups.
- The WhatsApp app on the Mac, Messenger, Apple Mail, and your phone are outside this block. Mail keeps working because `imap.gmail.com` is not listed.
- `google.com` and `accounts.google.com` are not listed, so signing in to Google still works.

## Tests

```bash
make test
```

## Remove it

Stop the helper first, then delete the block it wrote. If you only stop the helper, the names stay in `/etc/hosts` and the sites stay closed.

```bash
sudo launchctl bootout system/com.akrem.time-protector
sudo launchctl disable system/com.akrem.time-protector
```

Open `/etc/hosts` as root and delete the lines from `# BEGIN time-protector` through `# END time-protector`, including those two marker lines. Then:

```bash
sudo dscacheutil -flushcache
sudo killall -HUP mDNSResponder
sudo rm /Library/LaunchDaemons/com.akrem.time-protector.plist
sudo rm /usr/local/bin/time-protector
```

The hours file and the hosts backup stay in `/usr/local/etc/time-protector` and `/usr/local/var/time-protector` until you delete those folders yourself.

## License

MIT. See [LICENSE](LICENSE).
