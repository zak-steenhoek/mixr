package mixr

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareReleaseLogArchivesCurrentAndRetainsFive(t *testing.T) {
	directory, err := os.MkdirTemp(".", ".logger-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	for index := 0; index < retainedRunLogs; index++ {
		name := fmt.Sprintf("%s2026010%d-000000.000.log", runLogPrefix, index+1)
		if err := os.WriteFile(filepath.Join(directory, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, logFilename), []byte("latest"), 0600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 30, 12, 34, 56, 789000000, time.UTC)
	if err := prepareReleaseLog(directory, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, logFilename)); !os.IsNotExist(err) {
		t.Fatalf("current log still exists after archive: %v", err)
	}
	archives, err := filepath.Glob(filepath.Join(directory, runLogPrefix+"*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != retainedRunLogs {
		t.Fatalf("archive count = %d, want %d", len(archives), retainedRunLogs)
	}
	archivedCurrent := filepath.Join(directory, runLogPrefix+now.Format(runLogTime)+".log")
	contents, err := os.ReadFile(archivedCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "latest" {
		t.Fatalf("archived contents = %q", contents)
	}
}
