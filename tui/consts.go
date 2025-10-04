package tui

import "time"

const (
	// Durations for transient message display
	errorToastDuration   = 5 * time.Second
	successToastDuration = 3 * time.Second

	// Buffer limits
	serverLogKeep = 100
	probeLogKeep  = 1000
)
