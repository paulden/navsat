package config

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Region != "eu-west-2" {
		t.Errorf("Region = %q, want eu-west-2", cfg.Region)
	}
	if cfg.InstanceType != "t4g.nano" {
		t.Errorf("InstanceType = %q, want t4g.nano", cfg.InstanceType)
	}
	if cfg.SOCKSPort != 9000 {
		t.Errorf("SOCKSPort = %d, want 9000", cfg.SOCKSPort)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := loadFrom("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	def := Default()
	if cfg.Region != def.Region {
		t.Errorf("Region = %q, want %q", cfg.Region, def.Region)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{
		Region:       "us-west-2",
		InstanceType: "t3.nano",
		SOCKSPort:    8080,
	}

	if err := saveTo(want, path); err != nil {
		t.Fatalf("saveTo: %v", err)
	}

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}

	if got != want {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	saved := Config{
		Region:       "ap-southeast-1",
		InstanceType: "t3.nano",
		SOCKSPort:    1234,
	}
	if err := saveTo(saved, path); err != nil {
		t.Fatalf("saveTo: %v", err)
	}

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if got.Region != "ap-southeast-1" {
		t.Errorf("Region = %q, want ap-southeast-1", got.Region)
	}
	if got.SOCKSPort != 1234 {
		t.Errorf("SOCKSPort = %d, want 1234", got.SOCKSPort)
	}
	if got.InstanceType != "t3.nano" {
		t.Errorf("InstanceType = %q, want t3.nano", got.InstanceType)
	}
}
