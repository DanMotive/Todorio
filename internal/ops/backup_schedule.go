// Scheduled backups: install a systemd timer that runs the very same
// `todorio backup create` an operator would type by hand, plus retention for
// what those runs leave behind.
//
// Why a timer and not a goroutine in internal/worker: the worker's tickers are
// created at process start, so their idea of "daily" drifts by however long the
// server was down or however often it was restarted (the comment above
// warnArchiveExpiring already says as much). A backup that quietly slides from
// 03:00 to the middle of the working day, or that never runs on a box that is
// restarted every evening, is worse than no backup, because `todorio status`
// would still report backups on disk. systemd already solves this: OnCalendar
// is wall-clock, and Persistent=true makes a run that was missed while the
// machine was off happen at the next boot instead of being skipped. Todorio is
// already a systemd product — install.sh writes the unit, and start/stop/restart
// shell out to systemctl — so this borrows the scheduler that is guaranteed to
// be there rather than shipping a second, worse one inside the process.
package ops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/term"
)

const (
	backupServiceName = "todorio-backup.service"
	backupTimerName   = "todorio-backup.timer"
	backupServicePath = "/etc/systemd/system/" + backupServiceName
	backupTimerPath   = "/etc/systemd/system/" + backupTimerName
)

// DefaultBackupKeep is how many backup sets survive a prune when the operator
// does not say otherwise. Seven daily sets is one week of history, which is the
// point at which "I deleted the wrong list on Monday" is still recoverable on
// Friday without the backups directory growing without bound.
const DefaultBackupKeep = 7

// BackupSchedule is a parsed `todorio backup schedule ...` invocation.
type BackupSchedule struct {
	// OnCalendar is a systemd calendar expression (systemd.time(7)).
	OnCalendar string
	// Human is the same thing in the words the operator typed, for messages.
	Human string
	// Keep is passed through to `backup create --prune --keep`.
	Keep int
}

var clockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)

// weekdays maps what a person types to what systemd expects. Both the short and
// the long form are accepted because there is no reason to make someone guess.
var weekdays = map[string]string{
	"mon": "Mon", "monday": "Mon",
	"tue": "Tue", "tues": "Tue", "tuesday": "Tue",
	"wed": "Wed", "weds": "Wed", "wednesday": "Wed",
	"thu": "Thu", "thur": "Thu", "thurs": "Thu", "thursday": "Thu",
	"fri": "Fri", "friday": "Fri",
	"sat": "Sat", "saturday": "Sat",
	"sun": "Sun", "sunday": "Sun",
}

// ParseBackupSchedule turns the CLI tail into a schedule.
//
//	daily 03:30
//	weekly sun 04:00
//	hourly
//
// plus an optional --keep <n> anywhere in the arguments. Kept free of any I/O so
// the argument handling can be tested without a systemd (or a machine) to hand.
func ParseBackupSchedule(args []string) (BackupSchedule, error) {
	s := BackupSchedule{Keep: DefaultBackupKeep}

	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--keep":
			if i+1 >= len(args) {
				return s, fmt.Errorf("--keep requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return s, fmt.Errorf("--keep expects a whole number of backup sets, got %q", args[i])
			}
			s.Keep = n
		default:
			if strings.HasPrefix(args[i], "-") {
				return s, fmt.Errorf("unknown flag for backup schedule: %s", args[i])
			}
			pos = append(pos, args[i])
		}
	}

	if len(pos) == 0 {
		return s, fmt.Errorf("say when to run: `daily HH:MM`, `weekly <day> HH:MM`, `hourly`, or `off`")
	}

	switch strings.ToLower(pos[0]) {
	case "hourly":
		if len(pos) != 1 {
			return s, fmt.Errorf("hourly takes no further arguments")
		}
		// On the hour is deliberate: a backup at :00 is easy to correlate with
		// everything else in the journal.
		s.OnCalendar = "hourly"
		s.Human = "every hour, on the hour"
	case "daily":
		if len(pos) != 2 {
			return s, fmt.Errorf("daily needs a time, e.g. `todorio backup schedule daily 03:30`")
		}
		if !clockRe.MatchString(pos[1]) {
			return s, fmt.Errorf("%q is not a 24-hour HH:MM time", pos[1])
		}
		s.OnCalendar = "*-*-* " + pos[1] + ":00"
		s.Human = "every day at " + pos[1]
	case "weekly":
		if len(pos) != 3 {
			return s, fmt.Errorf("weekly needs a day and a time, e.g. `todorio backup schedule weekly sun 04:00`")
		}
		day, okDay := weekdays[strings.ToLower(pos[1])]
		if !okDay {
			return s, fmt.Errorf("%q is not a day of the week (mon, tue, ... sun)", pos[1])
		}
		if !clockRe.MatchString(pos[2]) {
			return s, fmt.Errorf("%q is not a 24-hour HH:MM time", pos[2])
		}
		s.OnCalendar = day + " *-*-* " + pos[2] + ":00"
		s.Human = "every " + day + " at " + pos[2]
	default:
		return s, fmt.Errorf("unknown schedule %q — expected `daily HH:MM`, `weekly <day> HH:MM`, `hourly`, or `off`", pos[0])
	}
	return s, nil
}

// requireSystemd mirrors the check ServiceControl already makes, with a message
// that points at the manual alternative rather than just refusing.
func requireSystemd() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found — scheduled backups use a systemd timer. On a machine without systemd, run `todorio backup create --prune` from cron instead")
	}
	return nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// binaryPath is the absolute path baked into the unit's ExecStart.
//
// os.Executable() is right in every real case (the installed binary lives at
// /usr/local/bin/todorio and that is what the operator just ran), but a symlink
// is resolved so the unit does not depend on a link that may be repointed, and
// a relative or unresolvable path falls back to the install location rather
// than writing an ExecStart that systemd cannot start.
func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "/usr/local/bin/todorio"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if !filepath.IsAbs(exe) {
		return "/usr/local/bin/todorio"
	}
	return exe
}

// BackupScheduleSet writes (or rewrites) the timer and enables it.
func BackupScheduleSet(s BackupSchedule) error {
	if err := requireSystemd(); err != nil {
		return err
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		// Not fatal: the timer is still worth installing, and postgresql-client
		// may be installed later. But silence here would mean discovering it
		// from a failed run at 03:30.
		warn("pg_dump not found — install postgresql-client, or every scheduled run will fail")
	}

	exe := binaryPath()
	prune := fmt.Sprintf("--prune --keep %d", s.Keep)
	if s.Keep == 0 {
		prune = "" // keep everything; see PruneBackups
	}
	service := fmt.Sprintf(`# Written by `+"`todorio backup schedule`"+` — edits here are overwritten by that command.
[Unit]
Description=Todorio backup
After=postgresql.service
Wants=postgresql.service

[Service]
Type=oneshot
# Exactly the command an operator would run by hand, so a scheduled backup and a
# manual one cannot drift apart. Output lands in the journal:
#   journalctl -u %s
ExecStart=%s backup create %s
`, backupServiceName, exe, prune)

	timer := fmt.Sprintf(`# Written by `+"`todorio backup schedule`"+` — edits here are overwritten by that command.
[Unit]
Description=Todorio backup (%s)

[Timer]
OnCalendar=%s
# A run missed because the machine was off happens shortly after the next boot
# instead of being skipped until the following day.
Persistent=true
# Nudge the start off the exact second so a backup never competes with whatever
# else on the box also fires at 03:00.
RandomizedDelaySec=120
Unit=%s

[Install]
WantedBy=timers.target
`, s.Human, s.OnCalendar, backupServiceName)

	if err := os.WriteFile(backupServicePath, []byte(service), 0o644); err != nil {
		return fmt.Errorf("writing %s (run with sudo?): %w", backupServicePath, err)
	}
	if err := os.WriteFile(backupTimerPath, []byte(timer), 0o644); err != nil {
		return fmt.Errorf("writing %s (run with sudo?): %w", backupTimerPath, err)
	}
	if err := runSystemctl("daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl("enable", "--now", backupTimerName); err != nil {
		return err
	}
	ok("scheduled backups: " + s.Human)
	if s.Keep > 0 {
		ok(fmt.Sprintf("keeping the newest %d backup set(s) in %s", s.Keep, backupsDir))
	} else {
		warn("--keep 0 — nothing is ever deleted; watch the disk")
	}
	fmt.Println("  ", term.Cyan("Next run:"), "todorio backup schedule status")
	return nil
}

// BackupScheduleOff stops the timer and removes both units. Backups already on
// disk are left alone — turning off a schedule is not a request to delete data.
func BackupScheduleOff() error {
	if err := requireSystemd(); err != nil {
		return err
	}
	if _, err := os.Stat(backupTimerPath); err != nil {
		ok("scheduled backups were not enabled — nothing to do")
		return nil
	}
	// Best effort: a unit that is already stopped or was never enabled makes
	// these exit non-zero, which is not a failure of this command.
	_ = exec.Command("systemctl", "disable", "--now", backupTimerName).Run()
	_ = os.Remove(backupTimerPath)
	_ = os.Remove(backupServicePath)
	if err := runSystemctl("daemon-reload"); err != nil {
		return err
	}
	ok("scheduled backups turned off — existing backups in " + backupsDir + " were kept")
	return nil
}

// unitValue pulls a single `Key=value` out of a unit file we wrote ourselves.
func unitValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

// BackupScheduleStatus reports whether the timer exists and when it fires next.
func BackupScheduleStatus() error {
	fmt.Println(term.Bold("scheduled backups"))
	if _, err := os.Stat(backupTimerPath); err != nil {
		warn("not scheduled — `todorio backup schedule daily 03:30`")
		return nil
	}
	ok("timer: " + backupTimerPath)
	if v := unitValue(backupTimerPath, "OnCalendar"); v != "" {
		ok("runs: " + v)
	}
	if v := unitValue(backupServicePath, "ExecStart"); v != "" {
		ok("command: " + v)
	}
	if err := requireSystemd(); err != nil {
		return nil // the units are there to read; systemd is not, so stop here
	}
	fmt.Println()
	// list-timers is the honest answer to "when does it actually run next" —
	// better than re-deriving it from OnCalendar and getting it subtly wrong.
	return runSystemctl("list-timers", "--all", "--no-pager", backupTimerName)
}

// backupFile matches only the two names Backup() creates. Anything else in the
// backups directory — an operator's manual copy, a half-finished download, a
// note to self — is invisible to listing and, more importantly, to pruning.
var backupFile = regexp.MustCompile(`^(?:todorio-(\d{8}-\d{6})\.sql\.gz|uploads-(\d{8}-\d{6})\.tar\.gz)$`)

func backupStamp(name string) string {
	m := backupFile.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// backupSets groups filenames by their timestamp, newest first. The timestamp
// format is zero-padded and fixed-width, so sorting the strings sorts the dates.
func backupSets(names []string) (stamps []string, sets map[string][]string) {
	sets = map[string][]string{}
	for _, n := range names {
		ts := backupStamp(n)
		if ts == "" {
			continue
		}
		if _, seen := sets[ts]; !seen {
			stamps = append(stamps, ts)
		}
		sets[ts] = append(sets[ts], n)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(stamps)))
	for ts := range sets {
		sort.Strings(sets[ts])
	}
	return stamps, sets
}

// planPrune returns the files to delete so that only the newest `keep` sets
// remain. keep <= 0 means keep everything, so a mistyped or empty --keep can
// never be read as "delete the lot".
func planPrune(names []string, keep int) []string {
	if keep <= 0 {
		return nil
	}
	stamps, sets := backupSets(names)
	if len(stamps) <= keep {
		return nil
	}
	var doomed []string
	for _, ts := range stamps[keep:] {
		doomed = append(doomed, sets[ts]...)
	}
	sort.Strings(doomed)
	return doomed
}

func backupDirNames() ([]string, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// PruneBackups deletes every backup set except the newest `keep`.
func PruneBackups(keep int) error {
	names, err := backupDirNames()
	if err != nil {
		return fmt.Errorf("reading %s: %w", backupsDir, err)
	}
	if keep <= 0 {
		ok("retention is off (--keep 0) — nothing deleted")
		return nil
	}
	doomed := planPrune(names, keep)
	if len(doomed) == 0 {
		stamps, _ := backupSets(names)
		ok(fmt.Sprintf("%d backup set(s), keeping %d — nothing to delete", len(stamps), keep))
		return nil
	}
	var freed int64
	for _, n := range doomed {
		p := filepath.Join(backupsDir, n)
		if info, serr := os.Stat(p); serr == nil {
			freed += info.Size()
		}
		if err := os.Remove(p); err != nil {
			// One unreadable file should not stop the rest from being cleaned up.
			bad("could not delete " + p + ": " + err.Error())
			continue
		}
		ok("deleted " + n)
	}
	ok(fmt.Sprintf("pruned to the newest %d set(s), freed %.1f MB", keep, float64(freed)/(1<<20)))
	return nil
}

// BackupList prints what is on disk, newest first.
func BackupList() error {
	names, err := backupDirNames()
	if err != nil {
		return fmt.Errorf("reading %s: %w", backupsDir, err)
	}
	stamps, sets := backupSets(names)
	fmt.Println(term.Bold("backups"), "·", backupsDir)
	if len(stamps) == 0 {
		warn("no backups yet — `todorio backup create`")
		return nil
	}
	var total int64
	for _, ts := range stamps {
		var size int64
		for _, n := range sets[ts] {
			if info, serr := os.Stat(filepath.Join(backupsDir, n)); serr == nil {
				size += info.Size()
			}
		}
		total += size
		when := ts
		age := ""
		if t, perr := time.ParseInLocation("20060102-150405", ts, time.Local); perr == nil {
			when = t.Format("2006-01-02 15:04")
			age = fmt.Sprintf(" (%d day(s) ago)", int(time.Since(t).Hours()/24))
		}
		fmt.Printf("   %s  %6.1f MB  %s%s\n", term.Cyan(when), float64(size)/(1<<20), strings.Join(sets[ts], ", "), age)
	}
	fmt.Println()
	ok(fmt.Sprintf("%d backup set(s), %.1f MB total", len(stamps), float64(total)/(1<<20)))
	return nil
}
