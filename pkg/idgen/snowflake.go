// Package idgen provides ID generation utilities, including Snowflake ID generator.
// This package wraps github.com/sony/sonyflake to provide a consistent interface.
// Sonyflake is an improved version of Snowflake with longer lifespan and better scalability.
package idgen

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jxskiss/base62"
	"github.com/sony/sonyflake"
)

const (
	// Error message format for wrapping errors.
	errMsgFormat = "%w: %w"
)

// Config holds the settings for the ID generator.
type Config struct {
	// StartTime is the epoch of the ID generation (e.g., "2025-01-01T00:00:00Z").
	// Setting a custom StartTime extends the ID's lifespan.
	StartTime time.Time
	// MachineID is the unique identifier for the node (0-65535).
	// If 0, it will attempt to fetch from MACHINE_ID env or private IP.
	MachineID uint16
}

// Generator defines the interface for ID generation to support mocking.
type Generator interface {
	NextID() (uint64, error)
	NextShortID() (string, error)
	Parse(id uint64) (t time.Time, machineID uint16, seq uint16)
	ParseShortID(shortID string) (uint64, error)
}

// idGenerator implements the Generator interface using Sonyflake.
type idGenerator struct {
	sf        *sonyflake.Sonyflake
	startTime time.Time
}

// NewGenerator initializes a new Sonyflake generator.
// This function is intended to be used with Wire.
func NewGenerator(cfg *Config) (Generator, error) {
	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	settings := sonyflake.Settings{
		StartTime: cfg.StartTime,
	}

	// Set MachineID function if provided, otherwise use default (nil = auto-detect from private IP)
	if cfg.MachineID != 0 {
		settings.MachineID = func() (uint16, error) {
			return cfg.MachineID, nil
		}
	} else {
		// Try to get from environment variable first
		if idEnv := os.Getenv("MACHINE_ID"); idEnv != "" {
			if v, err := strconv.ParseUint(idEnv, 10, 16); err == nil {
				machineID := uint16(v)
				settings.MachineID = func() (uint16, error) {
					return machineID, nil
				}
			}
		}
		// If MachineID is nil, sonyflake will use default (lower 16 bits of private IP)
	}

	sf := sonyflake.NewSonyflake(settings)
	if sf == nil {
		return nil, fmt.Errorf("%w", ErrInvalidConfiguration)
	}

	return &idGenerator{
		sf:        sf,
		startTime: cfg.StartTime,
	}, nil
}

// NextID generates a raw uint64 ID. Best for database primary keys.
func (g *idGenerator) NextID() (uint64, error) {
	id, err := g.sf.NextID()
	if err != nil {
		return 0, fmt.Errorf(errMsgFormat, ErrIDGenerationFailed, err)
	}

	return id, nil
}

// NextShortID generates a Base62 encoded string.
// Excellent for mediaID, userID, or short URLs.
// The Base62 encoding of Sonyflake IDs provides high prefix scattering
// because MachineID and Sequence are in the lower bits.
func (g *idGenerator) NextShortID() (string, error) {
	id, err := g.sf.NextID()
	if err != nil {
		return "", fmt.Errorf(errMsgFormat, ErrIDGenerationFailed, err)
	}

	return string(base62.FormatUint(id)), nil
}

// Parse extracts metadata from a generated ID.
// Uses Sonyflake's built-in decomposition functions for accurate parsing.
func (g *idGenerator) Parse(id uint64) (t time.Time, machineID uint16, seq uint16) {
	// Use Sonyflake's built-in functions to extract components
	machineID = uint16(sonyflake.MachineID(id))
	seq = uint16(sonyflake.SequenceNumber(id))

	// Calculate timestamp from elapsed time
	elapsed := sonyflake.ElapsedTime(id)
	t = g.startTime.Add(elapsed)

	return
}

// ParseShortID parses a Base62 encoded ID back to uint64.
func (g *idGenerator) ParseShortID(shortID string) (uint64, error) {
	id, err := base62.ParseUint([]byte(shortID))
	if err != nil {
		return 0, fmt.Errorf("failed to parse short ID: %w", err)
	}

	return id, nil
}

// NewSnowflakeGeneratorFromDatacenterAndWorker creates a generator from datacenter and worker IDs.
// This is a convenience function that calculates the machine ID automatically.
// Machine ID = (datacenterID * 32) + workerID.
func NewSnowflakeGeneratorFromDatacenterAndWorker(datacenterID, workerID int64, epoch int64) (Generator, error) {
	if datacenterID < 0 || datacenterID > 31 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidDatacenterID, datacenterID)
	}
	if workerID < 0 || workerID > 31 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidWorkerID, workerID)
	}

	// Calculate machine ID: (datacenterID * 32) + workerID
	machineID := uint16((datacenterID * 32) + workerID)

	var startTime time.Time
	if epoch > 0 {
		startTime = time.Unix(epoch/1000, (epoch%1000)*1000000)
	}

	return NewGenerator(&Config{
		MachineID: machineID,
		StartTime: startTime,
	})
}
