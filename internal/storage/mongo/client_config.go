package mongo

import (
	"fmt"
	"strings"
	"time"

	internalerr "github.com/aspirit/xsocial/internal/errors"

	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// ClientConfig represents a MongoDB client configuration with all supported parameters.
type ClientConfig struct {
	// Connection settings
	URI        string `mapstructure:"uri" json:"uri"`
	Username   string `mapstructure:"username,omitempty" json:"username,omitempty"`
	Password   string `mapstructure:"password,omitempty" json:"password,omitempty"`
	AuthSource string `mapstructure:"auth_source,omitempty" json:"authSource,omitempty"`

	// Connection pool settings
	PoolSize    int           `mapstructure:"pool_size" json:"poolSize"`
	MinPoolSize int           `mapstructure:"min_pool_size" json:"minPoolSize"`
	MaxIdleTime time.Duration `mapstructure:"max_idle_time" json:"maxIdleTime"`

	// Timeout settings
	ConnectTimeout         time.Duration `mapstructure:"connect_timeout" json:"connectTimeout"`
	SocketTimeout          time.Duration `mapstructure:"socket_timeout" json:"socketTimeout"`
	ServerSelectionTimeout time.Duration `mapstructure:"server_selection_timeout" json:"serverSelectionTimeout"`

	// Read/Write preferences
	ReadPreference string `mapstructure:"read_preference" json:"readPreference"`
	WriteConcern   string `mapstructure:"write_concern" json:"writeConcern"`
	ReadConcern    string `mapstructure:"read_concern" json:"readConcern"`

	// Retry settings
	RetryWrites bool `mapstructure:"retry_writes" json:"retryWrites"`
	RetryReads  bool `mapstructure:"retry_reads" json:"retryReads"`

	// Compression settings
	Compressors []string `mapstructure:"compressors" json:"compressors"`
}

// Config is the MongoDB section of the configuration: one client (Primary) and
// the logical → physical database mapping.
type Config struct {
	Primary ClientConfig `mapstructure:"primary" json:"primary"`

	// DatabaseOverrides maps logical DB name (e.g. "xs_db") to physical DB name
	// used at runtime. Empty means no override; the logical name is used directly.
	// Intended for test/staging isolation without touching code constants.
	DatabaseOverrides map[string]string `mapstructure:"database_overrides,omitempty" json:"databaseOverrides,omitempty"`
}

// ResolveDatabase returns the physical DB name for a logical name, applying any
// override from DatabaseOverrides. Falls back to the logical name when no mapping is set.
func (m *Config) ResolveDatabase(logicalName string) string {
	if m == nil || len(m.DatabaseOverrides) == 0 {
		return logicalName
	}
	if physical, ok := m.DatabaseOverrides[logicalName]; ok && physical != "" {
		return physical
	}

	return logicalName
}

// Validate validates the MongoDB client configuration.
func (c *ClientConfig) Validate() error {
	if c.URI == "" {
		return fmt.Errorf("%w", internalerr.ErrMongoURIEmpty)
	}

	if err := c.validatePoolSettings(); err != nil {
		return err
	}

	if err := c.validateTimeoutSettings(); err != nil {
		return err
	}

	if err := c.validateReadPreference(); err != nil {
		return err
	}

	if err := c.validateWriteConcern(); err != nil {
		return err
	}

	if err := c.validateReadConcern(); err != nil {
		return err
	}

	if err := c.validateCompressors(); err != nil {
		return err
	}

	return nil
}

// validatePoolSettings validates connection pool settings.
func (c *ClientConfig) validatePoolSettings() error {
	if c.PoolSize < 0 {
		return fmt.Errorf("%w, got %d", internalerr.ErrPoolSizeNegative, c.PoolSize)
	}

	if c.MinPoolSize < 0 {
		return fmt.Errorf("%w, got %d", internalerr.ErrMinPoolSizeNegative, c.MinPoolSize)
	}

	if c.MinPoolSize > c.PoolSize && c.PoolSize > 0 {
		return fmt.Errorf("%w (%d > %d)", internalerr.ErrMinPoolSizeGreaterThanMax, c.MinPoolSize, c.PoolSize)
	}

	return nil
}

// validateTimeoutSettings validates timeout settings.
func (c *ClientConfig) validateTimeoutSettings() error {
	if c.MaxIdleTime < 0 {
		return fmt.Errorf("%w, got %v", internalerr.ErrMaxIdleTimeNegative, c.MaxIdleTime)
	}

	if c.ConnectTimeout < 0 {
		return fmt.Errorf("%w, got %v", internalerr.ErrConnectTimeoutNegative, c.ConnectTimeout)
	}

	if c.SocketTimeout < 0 {
		return fmt.Errorf("%w, got %v", internalerr.ErrSocketTimeoutNegative, c.SocketTimeout)
	}

	if c.ServerSelectionTimeout < 0 {
		return fmt.Errorf("%w, got %v", internalerr.ErrServerSelectionTimeoutNegative, c.ServerSelectionTimeout)
	}

	return nil
}

// validateReadPreference validates read preference setting.
func (c *ClientConfig) validateReadPreference() error {
	if c.ReadPreference == "" {
		return nil // Empty is valid, will use default
	}

	validPrefs := []string{"primary", "primaryPreferred", "secondary", "secondaryPreferred", "nearest"}
	if !c.isValidValue(c.ReadPreference, validPrefs) {
		return fmt.Errorf("%w: %s", internalerr.ErrInvalidReadPreference, c.ReadPreference)
	}

	return nil
}

// validateWriteConcern validates write concern setting.
func (c *ClientConfig) validateWriteConcern() error {
	if c.WriteConcern == "" {
		return nil // Empty is valid, will use default
	}

	validWCs := []string{"majority", "w1", "w2", "w3", "acknowledged", "unacknowledged"}
	if !c.isValidValue(c.WriteConcern, validWCs) {
		return fmt.Errorf("%w: %s", internalerr.ErrInvalidWriteConcern, c.WriteConcern)
	}

	return nil
}

// validateReadConcern validates read concern setting.
func (c *ClientConfig) validateReadConcern() error {
	if c.ReadConcern == "" {
		return nil // Empty is valid, will use default
	}

	validRCs := []string{"local", "available", "majority", "linearizable", "snapshot"}
	if !c.isValidValue(c.ReadConcern, validRCs) {
		return fmt.Errorf("%w: %s", internalerr.ErrInvalidReadConcern, c.ReadConcern)
	}

	return nil
}

// validateCompressors validates compressor settings.
func (c *ClientConfig) validateCompressors() error {
	if len(c.Compressors) == 0 {
		return nil // Empty is valid
	}

	validCompressors := []string{"snappy", "zlib", "zstd"}
	for _, comp := range c.Compressors {
		if !c.isValidValue(comp, validCompressors) {
			return fmt.Errorf("%w: %s", internalerr.ErrInvalidCompressor, comp)
		}
	}

	return nil
}

// isValidValue checks if a value is in the list of valid values (case-insensitive).
func (c *ClientConfig) isValidValue(value string, validValues []string) bool {
	for _, valid := range validValues {
		if strings.EqualFold(value, valid) {
			return true
		}
	}

	return false
}

// GetReadPreference returns the MongoDB read preference.
func (c *ClientConfig) GetReadPreference() (*readpref.ReadPref, error) {
	switch strings.ToLower(c.ReadPreference) {
	case "", "primary":
		return readpref.Primary(), nil
	case "primarypreferred":
		return readpref.PrimaryPreferred(), nil
	case "secondary":
		return readpref.Secondary(), nil
	case "secondarypreferred":
		return readpref.SecondaryPreferred(), nil
	case "nearest":
		return readpref.Nearest(), nil
	default:
		return nil, fmt.Errorf("%w: %s", internalerr.ErrInvalidReadPreference, c.ReadPreference)
	}
}

// GetWriteConcern returns the MongoDB write concern.
func (c *ClientConfig) GetWriteConcern() (*writeconcern.WriteConcern, error) {
	switch strings.ToLower(c.WriteConcern) {
	case "", "acknowledged":
		return writeconcern.Majority(), nil
	case "majority":
		return writeconcern.Majority(), nil
	case "w1":
		return writeconcern.W1(), nil
	case "w2":
		return &writeconcern.WriteConcern{W: 2}, nil
	case "w3":
		return &writeconcern.WriteConcern{W: 3}, nil
	case "unacknowledged":
		return &writeconcern.WriteConcern{W: 0}, nil
	default:
		return nil, fmt.Errorf("%w: %s", internalerr.ErrInvalidWriteConcern, c.WriteConcern)
	}
}

// GetReadConcern returns the MongoDB read concern.
func (c *ClientConfig) GetReadConcern() (*readconcern.ReadConcern, error) {
	switch strings.ToLower(c.ReadConcern) {
	case "", "local":
		return readconcern.Local(), nil
	case "available":
		return readconcern.Available(), nil
	case "majority":
		return readconcern.Majority(), nil
	case "linearizable":
		return readconcern.Linearizable(), nil
	case "snapshot":
		return readconcern.Snapshot(), nil
	default:
		return nil, fmt.Errorf("%w: %s", internalerr.ErrInvalidReadConcern, c.ReadConcern)
	}
}

// SetDefaults sets default values for the configuration.
func (c *ClientConfig) SetDefaults() {
	if c.PoolSize == 0 {
		c.PoolSize = 10 // Default pool size
	}
	if c.MinPoolSize == 0 {
		c.MinPoolSize = 0 // Default min pool size
	}
	if c.MaxIdleTime == 0 {
		c.MaxIdleTime = 30 * time.Second // Default max idle time
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second // Default connect timeout
	}
	if c.SocketTimeout == 0 {
		c.SocketTimeout = 0 // No timeout by default
	}
	if c.ServerSelectionTimeout == 0 {
		c.ServerSelectionTimeout = 30 * time.Second // Default server selection timeout
	}
	if c.ReadPreference == "" {
		c.ReadPreference = "primary" // Default read preference
	}
	if c.WriteConcern == "" {
		c.WriteConcern = "majority" // Default write concern
	}
	if c.ReadConcern == "" {
		c.ReadConcern = "local" // Default read concern
	}
	if c.AuthSource == "" && (c.Username != "" || c.Password != "") {
		c.AuthSource = "admin" // Default auth source
	}
	// RetryWrites and RetryReads default to true in driver v2
	c.RetryWrites = true
	c.RetryReads = true
}

// Clone creates a deep copy of the configuration.
func (c *ClientConfig) Clone() *ClientConfig {
	// Clone compressors slice
	compressors := make([]string, len(c.Compressors))
	copy(compressors, c.Compressors)

	return &ClientConfig{
		URI:                    c.URI,
		Username:               c.Username,
		Password:               c.Password,
		AuthSource:             c.AuthSource,
		PoolSize:               c.PoolSize,
		MinPoolSize:            c.MinPoolSize,
		MaxIdleTime:            c.MaxIdleTime,
		ConnectTimeout:         c.ConnectTimeout,
		SocketTimeout:          c.SocketTimeout,
		ServerSelectionTimeout: c.ServerSelectionTimeout,
		ReadPreference:         c.ReadPreference,
		WriteConcern:           c.WriteConcern,
		ReadConcern:            c.ReadConcern,
		RetryWrites:            c.RetryWrites,
		RetryReads:             c.RetryReads,
		Compressors:            compressors,
	}
}

// String returns a string representation of the config (without sensitive data).
func (c *ClientConfig) String() string {
	return fmt.Sprintf("MongoDB{URI: %s, PoolSize: %d, MaxIdleTime: %v, ReadPreference: %s, RetryWrites: %t, Compressors: %v}",
		c.MaskedURI(), c.PoolSize, c.MaxIdleTime, c.ReadPreference, c.RetryWrites, c.Compressors)
}

// MaskedURI returns the URI with credentials masked.
func (c *ClientConfig) MaskedURI() string {
	return maskURI(c.URI)
}

// maskURI masks sensitive information in the URI.
func maskURI(uri string) string {
	if strings.Contains(uri, "@") {
		parts := strings.Split(uri, "@")
		if len(parts) >= 2 {
			return "mongodb://***@" + parts[len(parts)-1]
		}
	}

	return uri
}
