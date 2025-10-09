package types

import (
	"time"

	"github.com/fujiwara/trabbits/amqp091"
)

// Client status constants
const (
	ClientStatusActive       = "active"
	ClientStatusShuttingDown = "shutting_down"
	ClientStatusDisconnected = "disconnected"
)

// ClientInfo represents information about a connected client
type ClientInfo struct {
	ID               string        `json:"id"`
	ClientAddress    string        `json:"client_address,omitempty"`
	User             string        `json:"user,omitempty"`
	VirtualHost      string        `json:"virtual_host,omitempty"`
	ClientBanner     string        `json:"client_banner,omitempty"`
	ClientProperties amqp091.Table `json:"client_properties,omitempty"`
	ConnectedAt      time.Time     `json:"connected_at,omitempty"`
	DisconnectedAt   time.Time     `json:"disconnected_at,omitempty"`
	Status           string        `json:"status"` // ClientStatusActive, ClientStatusShuttingDown, or ClientStatusDisconnected
	ShutdownReason   string        `json:"shutdown_reason,omitempty"`
	Stats            *StatsSummary `json:"stats,omitempty"`
}

// StatsSummary represents a summary of proxy statistics for API responses
type StatsSummary struct {
	TotalMethods   int64  `json:"total_methods"`
	ReceivedFrames int64  `json:"received_frames"`
	SentFrames     int64  `json:"sent_frames"`
	TotalFrames    int64  `json:"total_frames"`
	Duration       string `json:"duration"`
}

// FullStatsSummary represents complete proxy statistics including method breakdown
type FullStatsSummary struct {
	StartedAt      time.Time        `json:"started_at"`
	Methods        map[string]int64 `json:"methods"`
	TotalMethods   int64            `json:"total_methods"`
	ReceivedFrames int64            `json:"received_frames"`
	SentFrames     int64            `json:"sent_frames"`
	TotalFrames    int64            `json:"total_frames"`
	Duration       string           `json:"duration"`
}

// StatsSnapshot represents a point-in-time snapshot of proxy statistics
type StatsSnapshot struct {
	StartedAt      time.Time        `json:"started_at"`
	Methods        map[string]int64 `json:"methods"`
	TotalMethods   int64            `json:"total_methods"`
	ReceivedFrames int64            `json:"received_frames"`
	SentFrames     int64            `json:"sent_frames"`
	TotalFrames    int64            `json:"total_frames"`
	Duration       string           `json:"duration"`
}

// FullClientInfo represents complete information about a connected client including full stats
type FullClientInfo struct {
	ID               string            `json:"id"`
	ClientAddress    string            `json:"client_address,omitempty"`
	User             string            `json:"user,omitempty"`
	VirtualHost      string            `json:"virtual_host,omitempty"`
	ClientBanner     string            `json:"client_banner,omitempty"`
	ClientProperties amqp091.Table     `json:"client_properties,omitempty"`
	ConnectedAt      time.Time         `json:"connected_at,omitempty"`
	DisconnectedAt   time.Time         `json:"disconnected_at,omitempty"`
	Status           string            `json:"status"` // ClientStatusActive, ClientStatusShuttingDown, or ClientStatusDisconnected
	ShutdownReason   string            `json:"shutdown_reason,omitempty"`
	Stats            *FullStatsSummary `json:"stats,omitempty"`
}

// ProbeLogEntry represents a probe log entry
type ProbeLogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	ProxyID   string         `json:"proxy_id,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}
