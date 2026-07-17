package models

import (
	"time"

	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TaskStatus is the current state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusDispatched TaskStatus = "dispatched"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusFailed     TaskStatus = "failed"
)

type Task struct {
	ID                primitive.ObjectID       `bson:"_id,omitempty" json:"id"`
	UserID            primitive.ObjectID       `bson:"user_id"        json:"user_id"`
	ProjectID         primitive.ObjectID       `bson:"project_id"    json:"project_id"`
	Name              string                   `bson:"name"           json:"name"`
	TemplateName      string                   `bson:"template_name"  json:"template_name"`
	TemplateID        string                   `bson:"template_id"    json:"template_id"`
	Targets           []string                 `bson:"targets"        json:"targets"`
	Config            TaskConfig               `bson:"config"         json:"config"`
	Modules           map[string][]StagePlugin `bson:"modules,omitempty" json:"modules,omitempty"`
	Status            TaskStatus               `bson:"status"        json:"status"`
	RunID             string                   `bson:"run_id,omitempty" json:"run_id,omitempty"`
	Progress          *StageProgress           `bson:"progress,omitempty" json:"progress"`
	NodeID            string                   `bson:"node_id"       json:"node_id"`
	NodeIDs           []string                 `bson:"node_ids"      json:"node_ids"`
	Retries           int                      `bson:"retries"       json:"retries"`
	Error             string                   `bson:"error"         json:"error"`
	CreatedAt         time.Time                `bson:"created_at"    json:"created_at"`
	UpdatedAt         time.Time                `bson:"updated_at"    json:"updated_at"`
	StartedAt         *time.Time               `bson:"started_at"    json:"started_at"`
	DoneAt            *time.Time               `bson:"done_at"       json:"done_at"`
	AIAnalysisEnabled bool                     `bson:"ai_analysis_enabled" json:"ai_analysis_enabled"`
	AIAnalysisStatus  string                   `bson:"ai_analysis_status,omitempty" json:"ai_analysis_status,omitempty"`
	AIAnalysis        string                   `bson:"ai_analysis,omitempty" json:"ai_analysis,omitempty"`
	AIAnalysisError   string                   `bson:"ai_analysis_error,omitempty" json:"ai_analysis_error,omitempty"`
	AIAnalysisLog     []string                 `bson:"ai_analysis_log,omitempty" json:"ai_analysis_log,omitempty"`
	AIAnalyzedAt      *time.Time               `bson:"ai_analyzed_at,omitempty" json:"ai_analyzed_at,omitempty"`
	AIPentestEnabled  bool                     `bson:"ai_pentest_enabled" json:"ai_pentest_enabled"`
	AIPentestStatus   string                   `bson:"ai_pentest_status,omitempty" json:"ai_pentest_status,omitempty"`
	AIPentestOutput   string                   `bson:"ai_pentest_output,omitempty" json:"ai_pentest_output,omitempty"`
	AIPentestError    string                   `bson:"ai_pentest_error,omitempty" json:"ai_pentest_error,omitempty"`
	AIPentestLog      []string                 `bson:"ai_pentest_log,omitempty" json:"ai_pentest_log,omitempty"`
	AIPentestNodeID   string                   `bson:"ai_pentest_node_id,omitempty" json:"ai_pentest_node_id,omitempty"`
	Blacklist         []*scanv1.BlacklistRule  `bson:"-" json:"-"`
}

type StageProgress struct {
	Stage   string `bson:"stage"   json:"stage"`
	Percent int32  `bson:"percent" json:"percent"`
	Message string `bson:"message" json:"message"`
}

type TaskConfig struct {
	Stages []string          `bson:"stages" json:"stages"` // ["subdomain","port","httpx","nuclei"]
	Params map[string]string `bson:"params" json:"params"` // {"port.rate":"1000"}
}

type TaskProgress struct {
	TaskID  string `bson:"task_id" json:"task_id"`
	Stage   string `bson:"stage"   json:"stage"`
	Percent int32  `bson:"percent" json:"percent"`
	Message string `bson:"message" json:"message"`
}
