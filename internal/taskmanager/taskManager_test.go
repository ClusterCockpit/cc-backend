package taskmanager

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"2m", 2 * time.Minute, false},
		{"1h", 1 * time.Hour, false},
		{"10s", 10 * time.Second, false},
		{"invalid", 0, true},
		{"", 0, true}, // time.ParseDuration returns error for empty string
		{"0", 0, false},
	}

	for _, tt := range tests {
		got, err := parseDuration(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestCronFrequencyParsing(t *testing.T) {
	jsonStr := `{"commit-job-worker": "10m", "duration-worker": "5m", "footprint-worker": "1h"}`
	var keys CronFrequency
	err := json.Unmarshal([]byte(jsonStr), &keys)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if keys.CommitJobWorker != "10m" {
		t.Errorf("Expected 10m, got %s", keys.CommitJobWorker)
	}
	if keys.DurationWorker != "5m" {
		t.Errorf("Expected 5m, got %s", keys.DurationWorker)
	}
	if keys.FootprintWorker != "1h" {
		t.Errorf("Expected 1h, got %s", keys.FootprintWorker)
	}
}

func TestDecodeCronConfigDeprecatedFootprintWorker(t *testing.T) {
	keys, err := decodeCronConfig(json.RawMessage(`{"duration-worker": "5m", "footprint-worker": "10m"}`))
	if err != nil {
		t.Fatalf("existing configuration with footprint-worker must still load: %v", err)
	}
	if keys.DurationWorker != "5m" {
		t.Errorf("duration-worker = %q, want 5m", keys.DurationWorker)
	}

	if _, err := decodeCronConfig(json.RawMessage(`{"unknown-worker": "1m"}`)); err == nil {
		t.Error("unknown cron keys must still be rejected")
	}
}

func TestRegisterJobWorkersSchedulesNoFootprintTask(t *testing.T) {
	saved, savedKeys := s, Keys
	t.Cleanup(func() { s, Keys = saved, savedKeys })

	var err error
	if Keys, err = decodeCronConfig(json.RawMessage(`{"footprint-worker": "10m"}`)); err != nil {
		t.Fatal(err)
	}
	if s, err = gocron.NewScheduler(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })

	registerJobWorkers()
	// Only the duration and commit-job workers remain.
	if got := len(s.Jobs()); got != 2 {
		t.Errorf("registered %d job workers, want 2 (no footprint worker)", got)
	}
}
