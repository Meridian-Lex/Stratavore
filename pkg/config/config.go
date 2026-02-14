package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Database      DatabaseConfig      `mapstructure:"database"`
	Docker        DockerConfig        `mapstructure:"docker"`
	Daemon        DaemonConfig        `mapstructure:"daemon"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Security      SecurityConfig      `mapstructure:"security"`
	GitHub        GitHubConfig        `mapstructure:"github"`
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	PostgreSQL PostgreSQLConfig `mapstructure:"postgresql"`
	SQLite     SQLiteConfig     `mapstructure:"sqlite"`
}

// PostgreSQLConfig for main state database
type PostgreSQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int    `mapstructure:"max_conns"`
	MinConns int    `mapstructure:"min_conns"`
}

// SQLiteConfig for local cache
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// DockerConfig for infrastructure integration
type DockerConfig struct {
	APIGateway APIGatewayConfig `mapstructure:"api_gateway"`
	RabbitMQ   RabbitMQConfig   `mapstructure:"rabbitmq"`
	Ntfy       NtfyConfig       `mapstructure:"ntfy"` // Deprecated
	Telegram   TelegramConfig   `mapstructure:"telegram"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Qdrant     QdrantConfig     `mapstructure:"qdrant"`
}

// APIGatewayConfig for Gantry API gateway
type APIGatewayConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Enabled bool   `mapstructure:"enabled"`
}

// RabbitMQConfig for event messaging
type RabbitMQConfig struct {
	Host              string `mapstructure:"host"`
	Port              int    `mapstructure:"port"`
	User              string `mapstructure:"user"`
	Password          string `mapstructure:"password"`
	Exchange          string `mapstructure:"exchange"`
	PublisherConfirms bool   `mapstructure:"publisher_confirms"`
}

// NtfyConfig for notifications (deprecated - using Telegram)
type NtfyConfig struct {
	Host   string            `mapstructure:"host"`
	Port   int               `mapstructure:"port"`
	Topics map[string]string `mapstructure:"topics"`
}

// TelegramConfig for notifications
type TelegramConfig struct {
	Token  string `mapstructure:"token"`
	ChatID string `mapstructure:"chat_id"`
}

// PrometheusConfig for metrics
type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// QdrantConfig for vector storage (future)
type QdrantConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Enabled bool   `mapstructure:"enabled"`
}

// GitHubConfig for fleet PR visibility
type GitHubConfig struct {
	Token      string   `mapstructure:"token"`
	FleetRepos []string `mapstructure:"fleet_repos"`
}

// DaemonConfig for daemon-specific settings
type DaemonConfig struct {
	Port_GRPC          int    `mapstructure:"grpc_port"`
	Port_HTTP          int    `mapstructure:"http_port"`
	HeartbeatInterval  int    `mapstructure:"heartbeat_interval_seconds"`
	ReconcileInterval  int    `mapstructure:"reconcile_interval_seconds"`
	OutboxPollInterval int    `mapstructure:"outbox_poll_interval_seconds"`
	ShutdownTimeout    int    `mapstructure:"shutdown_timeout_seconds"`
	DataDir            string `mapstructure:"data_dir"`
}

// ObservabilityConfig for logging and tracing
type ObservabilityConfig struct {
	LogLevel       string `mapstructure:"log_level"`
	LogFormat      string `mapstructure:"log_format"` // json or console
	TracingEnabled bool   `mapstructure:"tracing_enabled"`
}

// SecurityConfig for authentication and encryption
type SecurityConfig struct {
	EnableMTLS      bool            `mapstructure:"enable_mtls"`
	CertFile        string          `mapstructure:"cert_file"`
	KeyFile         string          `mapstructure:"key_file"`
	CAFile          string          `mapstructure:"ca_file"`
	TokenSecretPath string          `mapstructure:"token_secret_path"`
	JoinTokenTTL    int             `mapstructure:"join_token_ttl_seconds"`
	AuthSecret      string          `mapstructure:"auth_secret"`
	RateLimit       RateLimitConfig `mapstructure:"rate_limit"`
}

// RateLimitConfig controls per-client request throttling
type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
	Burst             int `mapstructure:"burst"`
}

// LoadConfig loads configuration from file and environment
func LoadConfig() (*Config, error) {
	v := viper.New()

	// Set config name and paths
	v.SetConfigName("stratavore")
	v.SetConfigType("yaml")

	// Search paths
	homeDir, _ := os.UserHomeDir()
	v.AddConfigPath(filepath.Join(homeDir, ".config", "stratavore"))
	v.AddConfigPath("/etc/stratavore")
	v.AddConfigPath(".")

	// Environment variables
	v.SetEnvPrefix("STRATAVORE")
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found is OK, use defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Override with secrets from files if specified
	if cfg.Security.TokenSecretPath != "" {
		// Read token secret from file (e.g., Docker secret)
		// This is just a placeholder - implement as needed
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Database defaults
	v.SetDefault("database.postgresql.host", "localhost")
	v.SetDefault("database.postgresql.port", 5432)
	v.SetDefault("database.postgresql.database", "stratavore_state")
	v.SetDefault("database.postgresql.user", "stratavore")
	v.SetDefault("database.postgresql.sslmode", "prefer")
	v.SetDefault("database.postgresql.max_conns", 25)
	v.SetDefault("database.postgresql.min_conns", 5)

	homeDir, _ := os.UserHomeDir()
	v.SetDefault("database.sqlite.path", filepath.Join(homeDir, ".config", "stratavore", "stratavore.db"))

	// Docker defaults
	v.SetDefault("docker.api_gateway.host", "localhost")
	v.SetDefault("docker.api_gateway.port", 8000)
	v.SetDefault("docker.api_gateway.enabled", false)

	v.SetDefault("docker.rabbitmq.host", "localhost")
	v.SetDefault("docker.rabbitmq.port", 5672)
	v.SetDefault("docker.rabbitmq.user", "guest")
	v.SetDefault("docker.rabbitmq.password", "guest")
	v.SetDefault("docker.rabbitmq.exchange", "stratavore.events")
	v.SetDefault("docker.rabbitmq.publisher_confirms", true)

	v.SetDefault("docker.ntfy.host", "localhost")
	v.SetDefault("docker.ntfy.port", 2586)
	v.SetDefault("docker.ntfy.topics.status", "stratavore-status")
	v.SetDefault("docker.ntfy.topics.alerts", "stratavore-alerts")

	// Telegram defaults (read from environment)
	v.SetDefault("docker.telegram.token", "")
	v.SetDefault("docker.telegram.chat_id", "")

	v.SetDefault("docker.prometheus.enabled", true)
	v.SetDefault("docker.prometheus.port", 9091)
	v.SetDefault("docker.prometheus.path", "/metrics")

	v.SetDefault("docker.qdrant.host", "localhost")
	v.SetDefault("docker.qdrant.port", 6333)
	v.SetDefault("docker.qdrant.enabled", false)

	// Daemon defaults
	v.SetDefault("daemon.grpc_port", 50051)
	v.SetDefault("daemon.heartbeat_interval_seconds", 10)
	v.SetDefault("daemon.reconcile_interval_seconds", 30)
	v.SetDefault("daemon.outbox_poll_interval_seconds", 2)
	v.SetDefault("daemon.shutdown_timeout_seconds", 30)
	v.SetDefault("daemon.data_dir", filepath.Join(homeDir, ".local", "share", "stratavore"))

	// Observability defaults
	v.SetDefault("observability.log_level", "info")
	v.SetDefault("observability.log_format", "json")
	v.SetDefault("observability.tracing_enabled", false)

	// Security defaults
	v.SetDefault("security.enable_mtls", false)
	v.SetDefault("security.join_token_ttl_seconds", 300)
	v.SetDefault("security.auth_secret", "") // empty = auth disabled
	v.SetDefault("security.rate_limit.requests_per_minute", 300)
	v.SetDefault("security.rate_limit.burst", 50)

	// GitHub defaults
	v.SetDefault("github.token", "")
	v.SetDefault("github.fleet_repos", []string{
		"Meridian-Lex/Stratavore",
		"Meridian-Lex/Gantry",
		"Meridian-Lex/Lex-webui",
		"Meridian-Lex/lex-state",
	})
}

// GetConnectionString returns PostgreSQL connection string
func (c *PostgreSQLConfig) GetConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
		c.SSLMode,
	)
}
