// Package config provides configuration management for the application.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/pkg/logger"
	telemetry "github.com/aspirit/xsocial/pkg/otelutil"
	"github.com/aspirit/xsocial/pkg/project"
	"github.com/aspirit/xsocial/pkg/ratelimit"
	"github.com/aspirit/xsocial/pkg/server"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// Project mode constants.
const (
	ModeDebug   = "debug"   // Development mode - enables debug features like Swagger docs
	ModeRelease = "release" // Production mode - disables debug features
)

// Config represents the application configuration.
type Config struct {
	Version  string             `mapstructure:"version"`
	Meta     MetaConfig         `mapstructure:"meta"`
	Project  ProjectConfig      `mapstructure:"project"`
	Features map[string]Feature `mapstructure:"features"`
	Services struct {
		API    server.ServerConfig `mapstructure:"api"`
		Worker server.ServerConfig `mapstructure:"worker"`
	} `mapstructure:"services"`
	// WorkerJobs holds worker-specific job configuration (poll intervals, batch sizes).
	WorkerJobs WorkerJobsConfig    `mapstructure:"worker_jobs"`
	Logging    logger.LoggerConfig `mapstructure:"logging"`
	Storage    struct {
		GCP                StorageGCPConfig         `mapstructure:"gcp"`
		MongoDB            mongo.Config             `mapstructure:"mongodb"`
		Redis              RedisConfig              `mapstructure:"redis"`
		Cache              CacheConfig              `mapstructure:"cache"`
		Kafka              KafkaConfig              `mapstructure:"kafka"`
		NotificationBroker NotificationBrokerConfig `mapstructure:"notification_broker"`
	} `mapstructure:"storage"`
	IDGen struct {
		Snowflake SnowflakeConfig `mapstructure:"snowflake"`
	} `mapstructure:"idgen"`
	Secrets struct {
		JWT JWTConfig `mapstructure:"jwt"`
	} `mapstructure:"secrets"`
	Otel      OtelConfig      `mapstructure:"otel"`
	RateLimit RateLimitConfig `mapstructure:"ratelimit"`
}

// MetaConfig holds configuration metadata.
type MetaConfig struct {
	LastUpdated time.Time `mapstructure:"last_updated"`
	Description string    `mapstructure:"description"`
	Owner       string    `mapstructure:"owner"`
}

// WorkerJobsConfig holds worker job-specific configuration.
type WorkerJobsConfig struct {
	Notification struct {
		PollInterval time.Duration `mapstructure:"poll_interval"`
		BatchSize    int           `mapstructure:"batch_size"`
		// WhoMaxSize caps the who array at N actors (e.g. 10). Extra count stored in who_count.
		WhoMaxSize int `mapstructure:"who_max_size"`
	} `mapstructure:"notification"`

	// Reconcile configures the periodic counter-reconciliation job.
	Reconcile struct {
		// Enabled turns the reconciliation scheduler on.
		Enabled bool `mapstructure:"enabled"`
		// Interval between reconciliation runs (e.g. "1h"); defaults to 1h when unset.
		Interval time.Duration `mapstructure:"interval"`
		// Correct, when true, repairs detected drift; when false, only reports it.
		Correct bool `mapstructure:"correct"`
	} `mapstructure:"reconcile"`
}

// ProjectConfig holds project-wide settings.
type ProjectConfig struct {
	ID   string `mapstructure:"id"`
	Mode string `mapstructure:"mode"`
}

// Feature holds feature flag configuration.
type Feature struct {
	Enabled           bool `mapstructure:"enabled"`
	RolloutPercentage int  `mapstructure:"rollout_percentage"`
}

// SnowflakeConfig holds configuration for Snowflake ID generator.
type SnowflakeConfig struct {
	// MachineID is the unique machine identifier (0-65535).
	// If 0, it will attempt to fetch from MACHINE_ID env or private IP.
	// Can also be calculated from datacenter_id and worker_id: (datacenter_id * 32) + worker_id
	MachineID uint16 `mapstructure:"machine_id" json:"machineId" yaml:"machine_id"`

	// DatacenterID is the datacenter identifier (0-31).
	// Used together with WorkerID to calculate MachineID if MachineID is not set.
	// In a distributed system, each datacenter should have a unique ID.
	DatacenterID int64 `mapstructure:"datacenter_id" json:"datacenterId" yaml:"datacenter_id"`

	// WorkerID is the worker/machine identifier (0-31).
	// Used together with DatacenterID to calculate MachineID if MachineID is not set.
	// Within a datacenter, each worker should have a unique ID.
	WorkerID int64 `mapstructure:"worker_id" json:"workerId" yaml:"worker_id"`

	// StartTime is the custom epoch timestamp.
	// If not set, the default start time (2025-01-01) will be used.
	// Format: RFC3339 string (e.g., "2025-01-01T00:00:00Z")
	StartTime string `mapstructure:"start_time,omitempty" json:"startTime,omitempty" yaml:"start_time,omitempty"`

	// Epoch is the custom epoch timestamp in milliseconds (deprecated, use StartTime instead).
	// If not set, the default epoch (2025-01-01) will be used.
	// Optional: defaults to 2025-01-01.
	Epoch int64 `mapstructure:"epoch,omitempty" json:"epoch,omitempty" yaml:"epoch,omitempty"`
}

type RedisConfig struct {
	Address      string `mapstructure:"address"`
	Password     string `mapstructure:"password"`
	TLSEnabled   bool   `mapstructure:"tls_enabled"` // Required for GCP Memorystore in production
	Dbname       int    `mapstructure:"dbname"`
	PoolSize     int    `mapstructure:"pool_size"`
	MaxRetries   int    `mapstructure:"max_retries"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
	Timeout      string `mapstructure:"timeout"`
}

// RateLimitConfig holds rate limiting configuration for the API.
type RateLimitConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Storage string        `mapstructure:"storage"` // ratelimit.StorageMemory | ratelimit.StorageCache (pkg/ratelimit)
	Global  RateLimitRule `mapstructure:"global"`
	// IPHardLimitMultiplier caps how much a single IP may exceed a per-device
	// policy limit, so churning X-Device-ID cannot bypass device-scoped policies
	// (distributed brute force). The per-IP ceiling is policy.Requests * this.
	// Defaults to 5 when <= 0.
	IPHardLimitMultiplier int               `mapstructure:"ip_hard_limit_multiplier"`
	Policies              []RateLimitPolicy `mapstructure:"policies"`
}

// RateLimitRule holds a single rate limit rule (e.g. global or one policy).
type RateLimitRule struct {
	Requests int    `mapstructure:"requests"`
	Window   string `mapstructure:"window"`
}

// RateLimitPolicy holds a path-specific rate limit policy.
// Only name and requests are required in config; method, pattern, match_type, key_mode come from internal/ratelimit policy defs.
// Window is optional; if empty, the default from the policy def is used.
type RateLimitPolicy struct {
	Name      string `mapstructure:"name"`
	Method    string `mapstructure:"-"` // filled from policy def
	Pattern   string `mapstructure:"-"` // filled from policy def
	MatchType string `mapstructure:"-"` // filled from policy def
	KeyMode   string `mapstructure:"-"` // filled from policy def
	Requests  int    `mapstructure:"requests"`
	Window    string `mapstructure:"window"`
}

// CacheConfig holds cache backend configuration.
// Type: redis (local) or memorystore (GCP). Both use Redis protocol.
// Uses storage.redis for connection when type is redis or memorystore.
type CacheConfig struct {
	Type string `mapstructure:"type"` // redis | memorystore
}

// KafkaConfig holds Kafka configuration.
// Topic names are fixed in internal/storage/eventbroker/events.go.
type KafkaConfig struct {
	Brokers  []string      `mapstructure:"brokers"`
	Security KafkaSecurity `mapstructure:"security"`
}

// KafkaSecurity holds Kafka security configuration.
type KafkaSecurity struct {
	TLSEnabled bool       `mapstructure:"tlsEnabled"`
	SASL       SASLConfig `mapstructure:"sasl"`
}

// SASLConfig holds SASL configuration.
type SASLConfig struct {
	Mechanism string `mapstructure:"mechanism"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
}

// JWTConfig holds JWT configuration.
type JWTConfig struct {
	Key string `mapstructure:"key"`
	// PreviousKeys holds keys retired during rotation. Tokens signed by them are
	// still accepted (parsed) until they expire, but new tokens are always signed
	// with Key. Drop a key from this list once its longest TTL has elapsed.
	PreviousKeys    []string `mapstructure:"previous_keys"`
	AccessTokenTTL  string   `mapstructure:"access_token_ttl"`
	RefreshTokenTTL string   `mapstructure:"refresh_token_ttl"`
	// Parsed TTL durations (set during config loading)
	ParsedAccessTokenTTL  time.Duration `mapstructure:"-"`
	ParsedRefreshTokenTTL time.Duration `mapstructure:"-"`
}

// StorageGCPConfig holds shared GCP configuration (Pub/Sub).
// Single source: storage.gcp.project_id (env: GCP_PROJECT_ID).
type StorageGCPConfig struct {
	ProjectID string `mapstructure:"project_id"`
}

// NotificationBrokerConfig holds notification event broker configuration (pkg/eventbroker).
// Type selects implementation: kafka (local) or pubsub (production).
// Topic names are fixed in internal/storage/eventbroker/events.go.
type NotificationBrokerConfig struct {
	Type               string `mapstructure:"type"`                // kafka | pubsub
	PubSubProjectID    string `mapstructure:"pubsub_project_id"`   // GCP project ID (required for pubsub)
	PubSubSubscription string `mapstructure:"pubsub_subscription"` // Pull subscription ID (default: notification-worker)
}

// OtelConfig holds OpenTelemetry configuration.
type OtelConfig struct {
	Endpoint string           `mapstructure:"endpoint"`
	Exporter string           `mapstructure:"exporter"`
	Trace    OtelTraceConfig  `mapstructure:"trace"`
	Metric   OtelMetricConfig `mapstructure:"metric"`
	Log      OtelLogConfig    `mapstructure:"log"`
}

// OtelTraceConfig holds tracing configuration.
type OtelTraceConfig struct {
	Enabled       bool    `mapstructure:"enabled"`
	SamplingRatio float64 `mapstructure:"sampling_ratio"`
}

// OtelMetricConfig holds metric configuration.
type OtelMetricConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	PrometheusPort int    `mapstructure:"prometheus_port"`
	PrometheusHost string `mapstructure:"prometheus_host"` // bind host for /metrics; empty = all interfaces. Set 127.0.0.1 to restrict to loopback (else rely on firewall / k8s NetworkPolicy).
}

// OtelLogConfig holds log configuration.
type OtelLogConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

var (
	config Config
)

var (
	basicViper   *viper.Viper
	basicViperMu sync.Mutex
)

// init initializes the basic and usernames configurations.
func init() {
	LoadBasicConfig()
}

// expandEnvVars expands environment variables in a string, supporting ${VAR:-default} syntax.
func expandEnvVars(s string) string {
	// Pattern to match ${VAR:-default} or ${VAR}
	re := regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]+))?\}`)

	return re.ReplaceAllStringFunc(s, func(match string) string {
		matches := re.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		varName := matches[1]
		defaultValue := ""
		if len(matches) > 2 {
			defaultValue = matches[2]
		}
		value := os.Getenv(varName)
		if value == "" {
			return defaultValue
		}

		return value
	})
}

// readAndExpandConfig reads a config file and expands environment variables.
func readAndExpandConfig(configPath string) ([]byte, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}
	expanded := expandEnvVars(string(data))

	return []byte(expanded), nil
}

// setupViperDefaults sets default values for Viper configuration.
func setupViperDefaults(v *viper.Viper) {
	v.SetDefault("idgen.snowflake.datacenter_id", 1)
	v.SetDefault("idgen.snowflake.worker_id", 1)
	v.SetDefault("storage.notification_broker.type", "kafka")
	v.SetDefault("storage.cache.type", "redis")
	v.SetDefault("storage.redis.tls_enabled", false)
	v.SetDefault("ratelimit.enabled", false)
	v.SetDefault("ratelimit.storage", ratelimit.StorageMemory)
	v.SetDefault("ratelimit.global.requests", 120)
	v.SetDefault("ratelimit.global.window", "1m")
}

// getConfigDir returns the configuration directory path and adds it to Viper.
func getConfigDir(v *viper.Viper) string {
	projectRoot, err := project.Root()
	if err != nil {
		// Fallback to relative path if project root cannot be determined
		fmt.Fprintf(os.Stderr, "Warning: failed to find project root, using relative path: %v\n", err)
		configDir := "configs"
		v.AddConfigPath(configDir)

		return configDir
	}

	configDir := filepath.Join(projectRoot, "configs")
	v.AddConfigPath(configDir)

	return configDir
}

// loadConfigFile loads a config file with environment variable expansion, with fallback.
func loadConfigFile(v *viper.Viper, configPath string, configName string, useMerge bool) {
	data, err := readAndExpandConfig(configPath)
	if err == nil {
		var readErr error
		if useMerge {
			readErr = v.MergeConfig(bytes.NewReader(data))
		} else {
			readErr = v.ReadConfig(bytes.NewReader(data))
		}
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "failed to read config %s: %v\n", configPath, readErr)
		}

		return
	}

	// Fallback to original method if file read fails
	v.SetConfigName(configName)
	var readErr error
	if useMerge {
		readErr = v.MergeInConfig()
	} else {
		readErr = v.ReadInConfig()
	}
	if readErr != nil {
		var configFileNotFoundErr viper.ConfigFileNotFoundError
		if !errors.As(readErr, &configFileNotFoundErr) {
			fmt.Fprintf(os.Stderr, "failed to read config %s: %v\n", configName, readErr)
		}
	}
}

// loadEnvConfig loads environment-specific configuration file.
func loadEnvConfig(v *viper.Viper, configDir string) {
	env := os.Getenv("ENV")
	if env == "default" {
		return
	}

	envConfigPath := filepath.Join(configDir, "config."+env+".yaml")
	loadConfigFile(v, envConfigPath, "config."+env, true)
}

// initBasicViper initializes Viper for config.default.yaml and config.{ENV}.yaml.
// It can be called multiple times to reload config (useful for tests).
func initBasicViper() {
	basicViperMu.Lock()
	defer basicViperMu.Unlock()

	// Load .env file if it exists
	if err := gotenv.Load(); err != nil {
		// .env file not found is not an error, just log it
		fmt.Fprintf(os.Stderr, "Note: .env file not found or could not be loaded: %v\n", err)
	}

	basicViper = viper.New()
	basicViper.SetConfigType("yaml")

	// Set defaults before reading config files
	setupViperDefaults(basicViper)

	// Get configuration directory
	configDir := getConfigDir(basicViper)

	// Bind environment variables before reading config
	bindEnvVars(basicViper)

	// Load default config
	defaultConfigPath := filepath.Join(configDir, "config.default.yaml")
	loadConfigFile(basicViper, defaultConfigPath, "config.default", false)

	// Load environment-specific config
	loadEnvConfig(basicViper, configDir)
}

// bindEnvVars binds environment variables to viper configuration.
func bindEnvVars(v *viper.Viper) {
	envBindings := map[string]string{
		"storage.mongodb.primary.uri":      "MONGODB_URI_PRIMARY",
		"storage.mongodb.primary.username": "MONGODB_USERNAME_PRIMARY",
		"storage.mongodb.primary.password": "MONGODB_PASSWORD_PRIMARY",
		"storage.redis.address":            "CACHE_ADDRESS",
		"storage.redis.password":           "CACHE_PASSWORD",
		"storage.redis.tls_enabled":        "CACHE_TLS_ENABLED",
		"storage.cache.type":               "CACHE_TYPE",
		"storage.kafka.brokers":            "KAFKA_BROKERS",
		"storage.gcp.project_id":           "GCP_PROJECT_ID",
		"secrets.jwt.key":                  "JWT_KEY",
		"otel.endpoint":                    "OTEL_ENDPOINT",
		// Snowflake ID generator configuration
		"idgen.snowflake.datacenter_id": "SNOWFLAKE_DATACENTER_ID",
		"idgen.snowflake.worker_id":     "SNOWFLAKE_WORKER_ID",
		// Notification broker (Kafka local, Pub/Sub production)
		"storage.notification_broker.type":                "NOTIFICATION_BROKER",
		"storage.notification_broker.pubsub_subscription": "PUBSUB_SUBSCRIPTION",
	}

	for configKey, envVar := range envBindings {
		if err := v.BindEnv(configKey, envVar); err != nil {
			fmt.Fprintf(os.Stderr, "failed to bind %s env: %v\n", envVar, err)
		}
	}
}

// LoadBasicConfig loads configurations from config.default.yaml and config.{ENV}.yaml.
// It can be called multiple times to reload config (useful for tests).
func LoadBasicConfig() *Config {
	initBasicViper()
	if err := basicViper.Unmarshal(&config); err != nil {
		fmt.Fprintf(os.Stderr, "failed to unmarshal config: %v\n", err)
	}

	// Apply defaults for snowflake config if values are invalid (e.g., when config file has ${VAR:-default} syntax)
	// This handles cases where environment variables are not set and config file has placeholder strings
	if config.IDGen.Snowflake.DatacenterID <= 0 {
		config.IDGen.Snowflake.DatacenterID = 1
	}
	if config.IDGen.Snowflake.WorkerID <= 0 {
		config.IDGen.Snowflake.WorkerID = 1
	}

	// Parse JWT TTL durations during config loading
	if err := parseJWTTTLDurations(&config.Secrets.JWT); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse JWT TTL durations: %v\n", err)
		os.Exit(1) // Exit immediately on TTL parsing error
	}

	// Drop empty previous_keys entries (e.g. unset JWT_KEY_PREVIOUS → "") so they
	// never enter the token verification set as a zero-entropy signing key.
	sanitizeJWTPreviousKeys(&config.Secrets.JWT)

	// Propagate storage.gcp.project_id to Pub/Sub when the subsection omits it
	applyGCPProjectID(&config)

	return &config
}

// sanitizeJWTPreviousKeys removes blank previous_keys entries in place.
// An empty string derives a public Ed25519 key (sha256("")) and must never be trusted.
func sanitizeJWTPreviousKeys(jwtConfig *JWTConfig) {
	if jwtConfig == nil || len(jwtConfig.PreviousKeys) == 0 {
		return
	}
	out := jwtConfig.PreviousKeys[:0]
	for _, k := range jwtConfig.PreviousKeys {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, k)
	}
	jwtConfig.PreviousKeys = out
}

// applyGCPProjectID propagates storage.gcp.project_id to the Pub/Sub notification broker.
func applyGCPProjectID(cfg *Config) {
	gcp := cfg.Storage.GCP.ProjectID
	if gcp == "" {
		return
	}
	if cfg.Storage.NotificationBroker.PubSubProjectID == "" {
		cfg.Storage.NotificationBroker.PubSubProjectID = gcp
	}
}

// parseJWTTTLDurations parses JWT TTL string durations into time.Duration values.
func parseJWTTTLDurations(jwtConfig *JWTConfig) error {
	// Parse access token TTL
	if jwtConfig.AccessTokenTTL != "" {
		accessTTL, err := time.ParseDuration(jwtConfig.AccessTokenTTL)
		if err != nil {
			return fmt.Errorf("invalid access_token_ttl duration '%s': %w", jwtConfig.AccessTokenTTL, err)
		}
		jwtConfig.ParsedAccessTokenTTL = accessTTL
	} else {
		jwtConfig.ParsedAccessTokenTTL = 1 * time.Hour // default fallback
	}

	// Parse refresh token TTL
	if jwtConfig.RefreshTokenTTL != "" {
		refreshTTL, err := time.ParseDuration(jwtConfig.RefreshTokenTTL)
		if err != nil {
			return fmt.Errorf("invalid refresh_token_ttl duration '%s': %w", jwtConfig.RefreshTokenTTL, err)
		}
		jwtConfig.ParsedRefreshTokenTTL = refreshTTL
	} else {
		jwtConfig.ParsedRefreshTokenTTL = 30 * 24 * time.Hour // default fallback
	}

	return nil
}

// GetConfig returns the loaded configuration.
func GetConfig() *Config {
	return &config
}

// GetOtelConfig returns the OpenTelemetry configuration.
func GetOtelConfig() telemetry.OtelInitConfig {
	telemetryConfig := telemetry.OtelInitConfig{
		ServiceName:    config.Services.API.ID,
		Environment:    config.Project.Mode,
		SampleRatio:    config.Otel.Trace.SamplingRatio,
		EnableTrace:    config.Otel.Trace.Enabled,
		EnableMetric:   config.Otel.Metric.Enabled,
		EnableLog:      config.Otel.Log.Enabled,
		TraceExporter:  config.Otel.Exporter,
		TraceEndpoint:  config.Otel.Endpoint,
		MetricExporter: config.Otel.Exporter,
		MetricEndpoint: config.Otel.Endpoint,
		PrometheusPort: config.Otel.Metric.PrometheusPort,
		PrometheusHost: config.Otel.Metric.PrometheusHost,
		Logger:         nil,
		LogLevel:       "info",
		Timeout:        5 * time.Second,
	}
	if config.Otel.Exporter == "" {
		telemetryConfig.TraceExporter = telemetry.ExporterOTLPgRPC
		telemetryConfig.MetricExporter = telemetry.ExporterOTLPgRPC
	}
	if config.Otel.Endpoint == "" {
		telemetryConfig.TraceEndpoint = "localhost:4317"
		telemetryConfig.MetricEndpoint = "localhost:4317"
	}
	if config.Otel.Log.Enabled {
		telemetryConfig.LogExporter = telemetry.ExporterNone
	}

	return telemetryConfig
}

// ValidateServiceConfig validates service configuration.
func ValidateServiceConfig(cfg *Config) error {
	services := map[string]server.ServerConfig{
		ServiceAPI:    cfg.Services.API,
		ServiceWorker: cfg.Services.Worker,
	}

	for name, service := range services {
		if service.ID == "" {
			return fmt.Errorf("service %s ID required: %w", name, internalerr.ErrServiceIDRequired)
		}

		// Validate ID format (should start with project prefix)
		expectedPrefix := cfg.Project.ID
		if !strings.HasPrefix(service.ID, expectedPrefix) {
			return fmt.Errorf("service %s ID '%s' should start with '%s': %w", name, service.ID, expectedPrefix, internalerr.ErrServiceIDInvalidPrefix)
		}

		if service.Addr == "" {
			return fmt.Errorf("service %s address required: %w", name, internalerr.ErrServiceAddressRequired)
		}
	}

	return nil
}

// JWT signing key strength thresholds. The Ed25519 private key is derived
// deterministically from this secret (see pkg/utils.deriveEd25519PrivateKey),
// so a low-entropy secret can be brute-forced offline to forge any token.
const (
	minJWTKeyLen           = 32
	minJWTKeyDistinctRunes = 12
)

// ValidateJWTKey checks that a JWT signing secret is long and diverse enough to
// resist offline brute-force recovery of the derived signing key.
func ValidateJWTKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return internalerr.ErrJWTKeyEmpty
	}
	if len(key) < minJWTKeyLen {
		return fmt.Errorf("%w: got %d bytes", internalerr.ErrJWTKeyTooShort, len(key))
	}
	distinct := make(map[rune]struct{}, len(key))
	for _, r := range key {
		distinct[r] = struct{}{}
	}
	if len(distinct) < minJWTKeyDistinctRunes {
		return fmt.Errorf("%w: only %d distinct characters", internalerr.ErrJWTKeyLowEntropy, len(distinct))
	}

	return nil
}

// ValidateJWTConfig validates the active JWT signing key and any rotation keys.
// Callers decide severity: fail-fast in release mode, warn in development.
func ValidateJWTConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if err := ValidateJWTKey(cfg.Secrets.JWT.Key); err != nil {
		return fmt.Errorf("secrets.jwt.key: %w", err)
	}
	for i, k := range cfg.Secrets.JWT.PreviousKeys {
		// Empties should already be stripped by sanitizeJWTPreviousKeys; any leftover
		// blank is rejected (zero-entropy keys must not enter the trust set).
		if err := ValidateJWTKey(k); err != nil {
			return fmt.Errorf("secrets.jwt.previous_keys[%d]: %w", i, err)
		}
	}

	return nil
}

// ValidateRateLimitEnabled enforces that rate limiting is on in production.
// Running release mode without rate limiting leaves auth endpoints unprotected,
// so this is a startup fail-fast.
func ValidateRateLimitEnabled(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if cfg.Project.Mode == ModeRelease && !cfg.RateLimit.Enabled {
		return internalerr.ErrRateLimitDisabledRelease
	}

	return nil
}

// ValidateRateLimitConfig validates ratelimit configuration when enabled.
// Resolve policies first (e.g. from API main via ResolveRateLimitPolicies) so method/pattern/match_type/key_mode are filled.
func ValidateRateLimitConfig(cfg *Config) error {
	if cfg == nil || !cfg.RateLimit.Enabled {
		return nil
	}
	rl := &cfg.RateLimit
	if err := validateRateLimitStorageAndGlobal(rl); err != nil {
		return err
	}
	for i := range rl.Policies {
		if err := validateRateLimitPolicy(&rl.Policies[i], i); err != nil {
			return err
		}
	}
	if rl.Storage == ratelimit.StorageCache && strings.TrimSpace(cfg.Storage.Redis.Address) == "" {
		return fmt.Errorf("ratelimit.storage is cache but storage.redis.address is empty (cache backend): %w", internalerr.ErrRateLimitCacheAddress)
	}

	return nil
}

func validateRateLimitStorageAndGlobal(rl *RateLimitConfig) error {
	switch rl.Storage {
	case ratelimit.StorageMemory, ratelimit.StorageCache:
	default:
		return fmt.Errorf("ratelimit.storage must be %q or %q, got %q: %w", ratelimit.StorageMemory, ratelimit.StorageCache, rl.Storage, internalerr.ErrRateLimitStorage)
	}
	if rl.Global.Requests <= 0 {
		return fmt.Errorf("ratelimit.global.requests must be positive, got %d: %w", rl.Global.Requests, internalerr.ErrRateLimitGlobalReq)
	}
	if _, err := time.ParseDuration(rl.Global.Window); err != nil {
		return fmt.Errorf("ratelimit.global.window invalid: %w", err)
	}

	return nil
}

func validateRateLimitPolicy(p *RateLimitPolicy, index int) error {
	if p.Name == "" {
		return fmt.Errorf("ratelimit.policies[%d].name required: %w", index, internalerr.ErrRateLimitPolicyName)
	}
	if p.Requests <= 0 {
		return fmt.Errorf("ratelimit.policies[%d].requests must be positive, got %d: %w", index, p.Requests, internalerr.ErrRateLimitPolicyReq)
	}
	if _, err := time.ParseDuration(p.Window); err != nil {
		return fmt.Errorf("ratelimit.policies[%d].window invalid: %w", index, err)
	}

	return nil
}
