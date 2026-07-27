package ops

import "testing"

func TestParseBackupSchedule(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCal    string
		wantKeep   int
		wantErrMsg bool
	}{
		{"daily", []string{"daily", "03:30"}, "*-*-* 03:30:00", DefaultBackupKeep, false},
		{"daily midnight", []string{"daily", "00:00"}, "*-*-* 00:00:00", DefaultBackupKeep, false},
		{"weekly short day", []string{"weekly", "sun", "04:00"}, "Sun *-*-* 04:00:00", DefaultBackupKeep, false},
		{"weekly long day", []string{"weekly", "Wednesday", "23:59"}, "Wed *-*-* 23:59:00", DefaultBackupKeep, false},
		{"hourly", []string{"hourly"}, "hourly", DefaultBackupKeep, false},
		{"keep before spec", []string{"--keep", "3", "daily", "01:00"}, "*-*-* 01:00:00", 3, false},
		{"keep after spec", []string{"daily", "01:00", "--keep", "30"}, "*-*-* 01:00:00", 30, false},
		{"keep zero", []string{"hourly", "--keep", "0"}, "hourly", 0, false},

		{"empty", nil, "", 0, true},
		{"unknown spec", []string{"fortnightly"}, "", 0, true},
		{"daily without time", []string{"daily"}, "", 0, true},
		{"24 is not a valid hour", []string{"daily", "24:00"}, "", 0, true},
		{"unpadded time", []string{"daily", "3:30"}, "", 0, true},
		{"minutes out of range", []string{"daily", "03:60"}, "", 0, true},
		{"weekly without a day", []string{"weekly", "04:00"}, "", 0, true},
		{"not a weekday", []string{"weekly", "funday", "04:00"}, "", 0, true},
		{"hourly with extras", []string{"hourly", "03:00"}, "", 0, true},
		{"keep without a value", []string{"daily", "03:00", "--keep"}, "", 0, true},
		{"keep is not a number", []string{"daily", "03:00", "--keep", "lots"}, "", 0, true},
		{"negative keep", []string{"daily", "03:00", "--keep", "-1"}, "", 0, true},
		{"unknown flag", []string{"daily", "03:00", "--force"}, "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBackupSchedule(tt.args)
			if tt.wantErrMsg {
				if err == nil {
					t.Fatalf("ParseBackupSchedule(%q) = %+v, want an error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBackupSchedule(%q): %v", tt.args, err)
			}
			if got.OnCalendar != tt.wantCal {
				t.Errorf("OnCalendar = %q, want %q", got.OnCalendar, tt.wantCal)
			}
			if got.Keep != tt.wantKeep {
				t.Errorf("Keep = %d, want %d", got.Keep, tt.wantKeep)
			}
			if got.Human == "" {
				t.Errorf("Human is empty — the confirmation message would read badly")
			}
		})
	}
}

func TestBackupStamp(t *testing.T) {
	matching := map[string]string{
		"todorio-20260727-031500.sql.gz": "20260727-031500",
		"uploads-20260727-031500.tar.gz": "20260727-031500",
	}
	for name, want := range matching {
		if got := backupStamp(name); got != want {
			t.Errorf("backupStamp(%q) = %q, want %q", name, got, want)
		}
	}
	// Anything the backup command did not write must stay unrecognised, because
	// unrecognised is what keeps it safe from prune.
	foreign := []string{
		"",
		"notes.txt",
		"todorio.sql.gz",
		"todorio-2026072-031500.sql.gz",
		"todorio-20260727-031500.sql",
		"todorio-20260727-031500.sql.gz.part",
		"my-todorio-20260727-031500.sql.gz",
		"uploads-20260727.tar.gz",
	}
	for _, name := range foreign {
		if got := backupStamp(name); got != "" {
			t.Errorf("backupStamp(%q) = %q, want \"\" (unrecognised files must not be prunable)", name, got)
		}
	}
}

func TestPlanPrune(t *testing.T) {
	names := []string{
		"todorio-20260725-030000.sql.gz", "uploads-20260725-030000.tar.gz",
		"todorio-20260726-030000.sql.gz", "uploads-20260726-030000.tar.gz",
		"todorio-20260727-030000.sql.gz", "uploads-20260727-030000.tar.gz",
		"README-restore.txt", // an operator's own file
	}

	t.Run("keeps the newest sets and deletes both halves of the older ones", func(t *testing.T) {
		got := planPrune(names, 2)
		want := []string{"todorio-20260725-030000.sql.gz", "uploads-20260725-030000.tar.gz"}
		if len(got) != len(want) {
			t.Fatalf("planPrune = %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("planPrune = %q, want %q", got, want)
			}
		}
	})

	t.Run("never touches files it does not recognise", func(t *testing.T) {
		for _, n := range planPrune(names, 1) {
			if n == "README-restore.txt" {
				t.Fatal("planPrune wanted to delete an unrelated file")
			}
		}
	})

	t.Run("nothing to do when there are fewer sets than kept", func(t *testing.T) {
		if got := planPrune(names, 3); got != nil {
			t.Fatalf("planPrune(keep=3) = %q, want nothing", got)
		}
		if got := planPrune(names, 99); got != nil {
			t.Fatalf("planPrune(keep=99) = %q, want nothing", got)
		}
	})

	t.Run("keep 0 means keep everything, not delete everything", func(t *testing.T) {
		if got := planPrune(names, 0); got != nil {
			t.Fatalf("planPrune(keep=0) = %q, want nothing", got)
		}
		if got := planPrune(names, -5); got != nil {
			t.Fatalf("planPrune(keep=-5) = %q, want nothing", got)
		}
	})

	t.Run("a dump without its uploads archive still counts as a set", func(t *testing.T) {
		lonely := []string{
			"todorio-20260101-000000.sql.gz",
			"todorio-20260102-000000.sql.gz",
		}
		got := planPrune(lonely, 1)
		if len(got) != 1 || got[0] != "todorio-20260101-000000.sql.gz" {
			t.Fatalf("planPrune = %q, want the older dump only", got)
		}
	})
}
