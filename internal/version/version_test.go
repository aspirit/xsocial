package version

import "testing"

func TestGet(t *testing.T) {
	info := Get()
	if info.Version != Version || info.Commit != Commit || info.BuildTime != BuildTime {
		t.Fatalf("Get() did not reflect package vars: %+v", info)
	}
	if info.APIVersion != APIVersion {
		t.Fatalf("APIVersion mismatch: got %d want %d", info.APIVersion, APIVersion)
	}
}

func TestGet_LdflagsOverride(t *testing.T) {
	// Defaults present when not built with ldflags.
	if Version == "" || Commit == "" || BuildTime == "" {
		t.Fatal("version vars must have non-empty defaults")
	}
}
