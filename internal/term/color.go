// Package term: minimal ANSI helpers for colored terminal output.
//
// Todorio's CLI intentionally uses plain words + color instead of emoji (some
// terminals, logging pipelines, and monitoring tools don't render emoji well,
// and antivirus/SIEM tooling sometimes flags them in scraped log output).
// Colors are skipped automatically when stdout isn't a real terminal (e.g. when
// piped into a log file or the systemd journal) or when NO_COLOR is set, so
// piped output stays plain and grep-friendly.
package term

import "os"

func isTTY(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

var enabled = isTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""

func wrap(code, s string) string {
	if !enabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// Green marks success/positive output.
func Green(s string) string { return wrap("32", s) }

// Yellow marks warnings.
func Yellow(s string) string { return wrap("33", s) }

// Red marks errors/failures.
func Red(s string) string { return wrap("31", s) }

// Cyan marks informational text.
func Cyan(s string) string { return wrap("36", s) }

// Bold marks headers/emphasis.
func Bold(s string) string { return wrap("1", s) }
