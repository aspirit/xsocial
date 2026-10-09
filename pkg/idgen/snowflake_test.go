package idgen

import (
	"testing"
	"time"
)

func TestNewGenerator(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config with machine ID",
			config: &Config{
				MachineID: 1,
			},
			wantErr: false,
		},
		{
			name: "valid config without machine ID (uses default)",
			config: &Config{
				MachineID: 0,
			},
			wantErr: false,
		},
		{
			name: "valid config with start time",
			config: &Config{
				MachineID: 1,
				StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "max valid machine ID",
			config: &Config{
				MachineID: 65535,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewGenerator(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewGenerator() expected error, got nil")
				}
				if gen != nil {
					t.Errorf("NewGenerator() expected nil generator on error")
				}
			} else {
				if err != nil {
					// MachineID 0 relies on MACHINE_ID env or private IP; skip when auto-detect fails (e.g. CI)
					if tt.config != nil && tt.config.MachineID == 0 {
						t.Skip("Skipping: no MACHINE_ID and auto-detect failed (e.g. in CI)")
					}

					t.Errorf("NewGenerator() unexpected error: %v", err)

					return
				}
				if gen == nil {
					t.Errorf("NewGenerator() expected generator, got nil")
				}
			}
		})
	}
}

func TestNewSnowflakeGeneratorFromDatacenterAndWorker(t *testing.T) {
	tests := []struct {
		name         string
		datacenterID int64
		workerID     int64
		wantErr      bool
	}{
		{
			name:         "valid IDs",
			datacenterID: 1,
			workerID:     1,
			wantErr:      false,
		},
		{
			name:         "max valid IDs",
			datacenterID: 31,
			workerID:     31,
			wantErr:      false,
		},
		{
			name:         "invalid datacenter ID - negative",
			datacenterID: -1,
			workerID:     1,
			wantErr:      true,
		},
		{
			name:         "invalid datacenter ID - too large",
			datacenterID: 32,
			workerID:     1,
			wantErr:      true,
		},
		{
			name:         "invalid worker ID - negative",
			datacenterID: 1,
			workerID:     -1,
			wantErr:      true,
		},
		{
			name:         "invalid worker ID - too large",
			datacenterID: 1,
			workerID:     32,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewSnowflakeGeneratorFromDatacenterAndWorker(tt.datacenterID, tt.workerID, 0)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewSnowflakeGeneratorFromDatacenterAndWorker() expected error, got nil")
				}
				if gen != nil {
					t.Errorf("NewSnowflakeGeneratorFromDatacenterAndWorker() expected nil generator on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewSnowflakeGeneratorFromDatacenterAndWorker() unexpected error: %v", err)

					return
				}
				if gen == nil {
					t.Errorf("NewSnowflakeGeneratorFromDatacenterAndWorker() expected generator, got nil")
				}
			}
		})
	}
}

func TestIDGenerator_NextID(t *testing.T) {
	gen, err := NewGenerator(&Config{
		MachineID: 1,
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Generate multiple IDs and verify they are unique
	ids := make(map[uint64]bool)
	count := 1000
	for i := 0; i < count; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("NextID() error = %v", err)
		}
		if ids[id] {
			t.Errorf("NextID() produced duplicate ID: %d", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("NextID() expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestIDGenerator_NextShortID(t *testing.T) {
	gen, err := NewGenerator(&Config{
		MachineID: 1,
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Generate multiple short IDs and verify they are unique
	ids := make(map[string]bool)
	count := 1000
	for i := 0; i < count; i++ {
		id, err := gen.NextShortID()
		if err != nil {
			t.Fatalf("NextShortID() error = %v", err)
		}
		if ids[id] {
			t.Errorf("NextShortID() produced duplicate ID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("NextShortID() expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestIDGenerator_Parse(t *testing.T) {
	gen, err := NewGenerator(&Config{
		MachineID: 100,
		StartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	id, err := gen.NextID()
	if err != nil {
		t.Fatalf("NextID() error = %v", err)
	}

	timestamp, machineID, sequence := gen.Parse(id)

	// Verify parsed components
	if machineID != 100 {
		t.Errorf("Parse() machineID = %d, want 100", machineID)
	}
	if sequence > 255 {
		t.Errorf("Parse() sequence = %d, want 0-255", sequence)
	}
	if timestamp.Before(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Parse() timestamp = %v, should be after 2025-01-01", timestamp)
	}
}

func TestIDGenerator_ParseShortID(t *testing.T) {
	gen, err := NewGenerator(&Config{
		MachineID: 1,
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Generate a short ID
	shortID, err := gen.NextShortID()
	if err != nil {
		t.Fatalf("NextShortID() error = %v", err)
	}

	// Parse it back
	parsedID, err := gen.ParseShortID(shortID)
	if err != nil {
		t.Fatalf("ParseShortID() error = %v", err)
	}

	// The parsed ID should be valid
	if parsedID == 0 {
		t.Errorf("ParseShortID() returned 0, expected valid ID")
	}

	// Generate another short ID and verify we can parse it
	anotherShortID, err := gen.NextShortID()
	if err != nil {
		t.Fatalf("NextShortID() error = %v", err)
	}

	// Verify we can parse the new short ID
	_, err = gen.ParseShortID(anotherShortID)
	if err != nil {
		t.Errorf("ParseShortID() failed on new ID: %v", err)
	}
}
