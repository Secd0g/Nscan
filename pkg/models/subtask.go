package models

import (
	"time"

	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SubtaskStatus 表示 subtask 的生命周期状态。
type SubtaskStatus string

const (
	SubtaskPending    SubtaskStatus = "pending"
	SubtaskLeased     SubtaskStatus = "leased"
	SubtaskDone       SubtaskStatus = "done"
	SubtaskFailed     SubtaskStatus = "failed"
	SubtaskDeadLetter SubtaskStatus = "dead_letter"
)

// Subtask 是 Task 被拆分后投入队列的最小执行单元。
// 一个 Task 的第一个 stage 会被拆成多个 Subtask 并发执行；
// 后续 stage 在上一 stage 全部完成后由 Aggregator 触发生成。
type Subtask struct {
	ID             string                  `bson:"_id" json:"id"`
	TaskID         primitive.ObjectID      `bson:"task_id" json:"task_id"`
	RunID          string                  `bson:"run_id" json:"run_id"`
	Stage          string                  `bson:"stage" json:"stage"`
	Capability     string                  `bson:"capability" json:"capability"` // 决定投入哪个队列，通常 = Stage
	Targets        []string                `bson:"targets" json:"targets"`
	Params         map[string]string       `bson:"params" json:"params"`
	Blacklist      []*scanv1.BlacklistRule `bson:"blacklist,omitempty" json:"blacklist,omitempty"`
	Attempt        int                     `bson:"attempt" json:"attempt"`
	Status         SubtaskStatus           `bson:"status" json:"status"`
	LeasedBy       string                  `bson:"leased_by" json:"leased_by"` // nodeID
	LeasedAt       time.Time               `bson:"leased_at" json:"leased_at"`
	LeaseExpiresAt time.Time               `bson:"lease_expires_at" json:"lease_expires_at"`
	OutputAssets   []byte                  `bson:"output_assets,omitempty" json:"-"` // 上报的资产 JSON
	ErrorMsg       string                  `bson:"error_msg,omitempty" json:"error_msg,omitempty"`
	CreatedAt      time.Time               `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time               `bson:"updated_at" json:"updated_at"`
	Extra          map[string]interface{}  `bson:"extra,omitempty" json:"-"` // 扩展字段
}
