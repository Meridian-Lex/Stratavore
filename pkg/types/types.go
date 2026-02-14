package types

import "time"

// RunnerStatus represents the state of a runner
type RunnerStatus string

const (
	StatusStarting   RunnerStatus = "starting"
	StatusRunning    RunnerStatus = "running"
	StatusPaused     RunnerStatus = "paused"
	StatusTerminated RunnerStatus = "terminated"
	StatusFailed     RunnerStatus = "failed"
)

// RuntimeType represents how the runner is executed
type RuntimeType string

const (
	RuntimeProcess   RuntimeType = "process"
	RuntimeContainer RuntimeType = "container"
	RuntimeRemote    RuntimeType = "remote"
)

// ProjectStatus represents project state
type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectIdle     ProjectStatus = "idle"
	ProjectArchived ProjectStatus = "archived"
)

// ConversationMode represents session continuation
type ConversationMode string

const (
	ModeNew      ConversationMode = "new"
	ModeContinue ConversationMode = "continue"
	ModeResume   ConversationMode = "resume"
)

// Runner represents a Meridian Lex instance
type Runner struct {
	ID           string       `json:"id"`
	RuntimeType  RuntimeType  `json:"runtime_type"`
	RuntimeID    string       `json:"runtime_id"`
	NodeID       string       `json:"node_id,omitempty"`
	ProjectName  string       `json:"project_name"`
	ProjectPath  string       `json:"project_path"`
	Status       RunnerStatus `json:"status"`
	Flags        []string     `json:"flags"`
	Capabilities []string     `json:"capabilities"`
	Environment  map[string]string `json:"environment"`
	
	SessionID        string           `json:"session_id,omitempty"`
	ConversationMode ConversationMode `json:"conversation_mode,omitempty"`
	SessionName      string           `json:"session_name,omitempty"`
	
	TokensUsed       int64   `json:"tokens_used"`
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryMB         int64   `json:"memory_mb"`
	
	RestartAttempts    int `json:"restart_attempts"`
	MaxRestartAttempts int `json:"max_restart_attempts"`
	
	StartedAt      time.Time  `json:"started_at"`
	LastHeartbeat  *time.Time `json:"last_heartbeat,omitempty"`
	HeartbeatTTL   int        `json:"heartbeat_ttl_seconds"`
	TerminatedAt   *time.Time `json:"terminated_at,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Project represents a development project
type Project struct {
	Name        string        `json:"name"`
	Path        string        `json:"path"`
	Status      ProjectStatus `json:"status"`
	Description string        `json:"description,omitempty"`
	Tags        []string      `json:"tags"`
	
	TotalRunners  int   `json:"total_runners"`
	ActiveRunners int   `json:"active_runners"`
	TotalSessions int   `json:"total_sessions"`
	TotalTokens   int64 `json:"total_tokens"`
	
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Session represents a conversation session
type Session struct {
	ID          string    `json:"id"`
	RunnerID    string    `json:"runner_id"`
	ProjectName string    `json:"project_name"`
	
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	LastMessageAt  *time.Time `json:"last_message_at,omitempty"`
	MessageCount   int        `json:"message_count"`
	TokensUsed     int64      `json:"tokens_used"`
	
	Resumable    bool   `json:"resumable"`
	ResumedFrom  string `json:"resumed_from,omitempty"`
	Summary      string `json:"summary,omitempty"`
	
	TranscriptS3Key   string `json:"transcript_s3_key,omitempty"`
	TranscriptSizeBytes int64 `json:"transcript_size_bytes,omitempty"`
	
	CreatedAt time.Time `json:"created_at"`
}

// Heartbeat represents agent health status
type Heartbeat struct {
	RunnerID   string       `json:"runner_id"`
	Status     RunnerStatus `json:"status"`
	Timestamp  time.Time    `json:"timestamp"`
	CPUPercent float64      `json:"cpu_percent"`
	MemoryMB   int64        `json:"memory_mb"`
	TokensUsed int64        `json:"tokens_used"`
	SessionID  string       `json:"session_id,omitempty"`
	
	// Agent metadata
	AgentVersion string `json:"agent_version"`
	Hostname     string `json:"hostname"`
}

// Event represents a system event for audit/event sourcing
type Event struct {
	ID         int64                  `json:"id"`
	EventID    string                 `json:"event_id"`
	Timestamp  time.Time              `json:"timestamp"`
	EventType  string                 `json:"event_type"`
	EntityType string                 `json:"entity_type"`
	EntityID   string                 `json:"entity_id"`
	Data       map[string]interface{} `json:"data"`
	Metadata   map[string]interface{} `json:"metadata"`
	UserID     string                 `json:"user_id,omitempty"`
	Hostname   string                 `json:"hostname,omitempty"`
	TraceID    string                 `json:"trace_id,omitempty"`
	Signature  string                 `json:"signature,omitempty"`
}

// OutboxEntry represents an event pending delivery
type OutboxEntry struct {
	ID            int64                  `json:"id"`
	CreatedAt     time.Time              `json:"created_at"`
	Delivered     bool                   `json:"delivered"`
	DeliveredAt   *time.Time             `json:"delivered_at,omitempty"`
	
	EventID       string                 `json:"event_id"`
	ServiceName   string                 `json:"service_name"`
	AggregateType string                 `json:"aggregate_type,omitempty"`
	AggregateID   string                 `json:"aggregate_id,omitempty"`
	EventType     string                 `json:"event_type"`
	
	Payload       map[string]interface{} `json:"payload"`
	Metadata      map[string]interface{} `json:"metadata"`
	RoutingKey    string                 `json:"routing_key"`
	
	Attempts      int        `json:"attempts"`
	MaxAttempts   int        `json:"max_attempts"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"`
	Error         string     `json:"error,omitempty"`
	
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

// LaunchRequest represents a request to start a runner
type LaunchRequest struct {
	ProjectName      string           `json:"project_name"`
	ProjectPath      string           `json:"project_path"`
	Flags            []string         `json:"flags"`
	Capabilities     []string         `json:"capabilities"`
	Environment      map[string]string `json:"environment"`
	ConversationMode ConversationMode `json:"conversation_mode"`
	SessionID        string           `json:"session_id,omitempty"`
	RuntimeType      RuntimeType      `json:"runtime_type"`
}

// ResourceQuota represents project resource limits
type ResourceQuota struct {
	ProjectName         string `json:"project_name"`
	MaxConcurrentRunners int   `json:"max_concurrent_runners"`
	MaxMemoryMB         int64  `json:"max_memory_mb,omitempty"`
	MaxCPUPercent       int    `json:"max_cpu_percent,omitempty"`
	MaxTokensPerDay     int64  `json:"max_tokens_per_day,omitempty"`
}

// TokenBudget represents token usage limits
type TokenBudget struct {
	ID                int       `json:"id"`
	Scope             string    `json:"scope"`
	ScopeID           string    `json:"scope_id,omitempty"`
	LimitTokens       int64     `json:"limit_tokens"`
	UsedTokens        int64     `json:"used_tokens"`
	PeriodGranularity string    `json:"period_granularity"`
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`
}

// DaemonInfo represents daemon state
type DaemonInfo struct {
	DaemonID      string                 `json:"daemon_id"`
	Hostname      string                 `json:"hostname"`
	Version       string                 `json:"version"`
	StartedAt     time.Time              `json:"started_at"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	Config        map[string]interface{} `json:"config"`
}

// Metrics represents global metrics
type Metrics struct {
	ActiveRunners  int   `json:"active_runners"`
	ActiveProjects int   `json:"active_projects"`
	TotalSessions  int   `json:"total_sessions"`
	TokensUsed     int64 `json:"tokens_used"`
	TokenLimit     int64 `json:"token_limit"`
}
