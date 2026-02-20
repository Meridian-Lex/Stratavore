package parsers

import "time"

// V2Project represents a project entry from PROJECT-MAP.md
type V2Project struct {
	Name     string
	Status   string // "ACTIVE" | "ARCHIVED" | "IDLE"
	Path     string
	Started  time.Time
	Priority string // "HIGH" | "MEDIUM" | "LOW" | "-"
	Notes    string
}

// V2Session represents a session entry from time_sessions.jsonl
type V2Session struct {
	SessionID   string    `json:"session_id"`
	ProjectName string    `json:"project_name"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	PausedTime  int64     `json:"paused_time_seconds"` // Total paused duration in seconds
	TokensUsed  int64     `json:"tokens_used"`
	Model       string    `json:"model,omitempty"`
	Summary     string    `json:"summary,omitempty"`
}

// V2Config represents LEX-CONFIG.yaml structure
type V2Config struct {
	Mode              V2Mode              `yaml:"mode"`
	Scheduling        V2Scheduling        `yaml:"scheduling"`
	AutonomousMode    V2AutonomousMode    `yaml:"autonomous_mode"`
	Development       V2Development       `yaml:"development"`
 // IDENTITY-EXCEPTION: functional internal reference — not for public exposure
	Claude            V2Claude            `yaml:"claude"`
	TokenBudget       V2TokenBudget       `yaml:"token_budget"`
	Notifications     V2Notifications     `yaml:"notifications"`
	Paths             V2Paths             `yaml:"paths"`
	GitHubMonitoring  V2GitHubMonitoring  `yaml:"github_monitoring"`
	Metadata          V2Metadata          `yaml:"metadata"`
}

// V2Mode represents operational mode
type V2Mode struct {
	Current     string `yaml:"current"` // "IDLE" | "AUTONOMOUS" | "DIRECTED" | "COLLABORATIVE"
	Description string `yaml:"description"`
}

// V2Scheduling represents scheduling configuration
type V2Scheduling struct {
	Enabled           bool   `yaml:"enabled"`
	TodoCheckInterval int    `yaml:"todo_check_interval"`
	LastCheck         string `yaml:"last_check"`
	NextCheck         string `yaml:"next_check"`
}

// V2AutonomousMode represents autonomous work settings
type V2AutonomousMode struct {
	Enabled         bool        `yaml:"enabled"`
	MaxDailyTokens  int64       `yaml:"max_daily_tokens"`
	WorkHours       V2WorkHours `yaml:"work_hours"`
	WorkPace        string      `yaml:"work_pace"` // "slow" | "steady" | "aggressive"
	Priorities      []string    `yaml:"priorities"`
}

// V2WorkHours represents work hour constraints
type V2WorkHours struct {
	Start string `yaml:"start"` // "HH:MM"
	End   string `yaml:"end"`   // "HH:MM"
}

// V2Development represents development practices
type V2Development struct {
	CommitStyle          string   `yaml:"commit_style"` // "conventional" | "semantic" | "descriptive"
	TestingRequired      bool     `yaml:"testing_required"`
	DocumentationRequired bool    `yaml:"documentation_required"`
	CodeReviewSelf       bool     `yaml:"code_review_self"`
	Standards            []string `yaml:"standards"`
}

// V2Claude represents Meridian Lex launch configuration
type V2Claude struct {
	DefaultFlags string         `yaml:"default_flags"`
	Presets      V2ClaudePresets `yaml:"presets"`
	QuickFlags   V2QuickFlags   `yaml:"quick_flags"`
	SystemPrompt V2SystemPrompt `yaml:"system_prompt"`
}

// V2ClaudePresets represents flag presets
type V2ClaudePresets struct {
	FullAccess string `yaml:"full-access"`
	PlanMode   string `yaml:"plan-mode"`
	AutoGit    string `yaml:"auto-git"`
}

// V2QuickFlags represents quick access flags
type V2QuickFlags struct {
	SkipPermissions bool `yaml:"skip_permissions"`
	AllowSkip       bool `yaml:"allow_skip"`
}

// V2SystemPrompt represents system prompt customization
type V2SystemPrompt struct {
	Append     string `yaml:"append"`
	AppendFile string `yaml:"append_file"`
}

// V2TokenBudget represents token budget configuration
type V2TokenBudget struct {
	DailyLimit             int64       `yaml:"daily_limit"`
	PerSessionTarget       int64       `yaml:"per_session_target"`
	ReservedForCommander   int64       `yaml:"reserved_for_commander"`
	Tracking               V2Tracking  `yaml:"tracking"`
}

// V2Tracking represents token tracking state
type V2Tracking struct {
	TodayUsed  int64  `yaml:"today_used"`
	LastReset  string `yaml:"last_reset"`
}

// V2Notifications represents notification settings
type V2Notifications struct {
	CompletionReports bool `yaml:"completion_reports"`
	ErrorAlerts       bool `yaml:"error_alerts"`
	ProgressUpdates   bool `yaml:"progress_updates"`
	DailySummary      bool `yaml:"daily_summary"`
}

// V2Paths represents path configuration
type V2Paths struct {
	Home            string `yaml:"home"`
	Projects        string `yaml:"projects"`
	Archive         string `yaml:"archive"`
	Docs            string `yaml:"docs"`
	Logs            string `yaml:"logs"`
	LexInternal     string `yaml:"lex_internal"`
	LexState        string `yaml:"lex_state"`
	LexConfig       string `yaml:"lex_config"`
	LexRegistry     string `yaml:"lex_registry"`
	StateMD         string `yaml:"state_md"`
	TaskQueue       string `yaml:"task_queue"`
	ProjectMap      string `yaml:"project_map"`
	TimeTracking    string `yaml:"time_tracking"`
	AutonomousLock  string `yaml:"autonomous_lock"`
	DockerServices  string `yaml:"docker_services"`
	Secrets         string `yaml:"secrets"`
	VesselStatus    string `yaml:"vessel_status"`
}

// V2GitHubMonitoring represents GitHub monitoring configuration
type V2GitHubMonitoring struct {
	Enabled      bool     `yaml:"enabled"`
	PollInterval int      `yaml:"poll_interval"`
	StateFile    string   `yaml:"state_file"`
	LogFile      string   `yaml:"log_file"`
	Repositories []string `yaml:"repositories"`
	TrackedUsers []string `yaml:"tracked_users"`
	Events       []string `yaml:"events"`
}

// V2Metadata represents configuration metadata
type V2Metadata struct {
	VesselID      string `yaml:"vessel_id"`
	Operator      string `yaml:"operator"`
	Commissioned  string `yaml:"commissioned"`
	ConfigVersion string `yaml:"config_version"`
}

// V2RankStatus represents rank-status.jsonl structure
type V2RankStatus struct {
	CurrentRank   string          `json:"current_rank"`
	Progress      string          `json:"progress"` // e.g., "2/5"
	Strikes       int             `json:"strikes"`
	Commendations int             `json:"commendations"`
	Events        []V2RankEvent   `json:"events"`
}

// V2RankEvent represents a rank progression event
type V2RankEvent struct {
	Type        string    `json:"type"` // "strike" | "commendation" | "promotion" | "demotion"
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Evidence    string    `json:"evidence,omitempty"`
}

// V2Directive represents a behavioral directive from behavioral-directives.jsonl
type V2Directive struct {
	ID               string                 `json:"id"`
	Timestamp        string                 `json:"timestamp,omitempty"`
	Severity         string                 `json:"severity"` // "PRIME" | "CRITICAL" | "HIGH" | "MEDIUM"
	TriggerCondition string                 `json:"trigger_condition"`
	Action           map[string]interface{} `json:"action"`
	DirectiveText    string                 `json:"directive_text"`
	StandardProcess  bool                   `json:"standard_process,omitempty"`
}
