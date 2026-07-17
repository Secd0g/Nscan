package models

import "time"

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
)

type Node struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Addr           string     `json:"addr"`
	Status         NodeStatus `json:"status"`
	Capabilities   []string   `json:"capabilities"`
	InstalledTools []string   `json:"installed_tools"`
	MaxTasks       int32      `json:"max_tasks"`
	ActiveTasks    int32      `json:"active_tasks"`
	CPUPercent     int32      `json:"cpu_percent"`
	MemPercent     int32      `json:"mem_percent"`
	Version        string     `json:"version"`
	RegisteredAt   time.Time  `json:"registered_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
}
