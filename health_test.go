// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits_test

import (
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
)

// Test that health check configuration is properly parsed
func TestHealthCheckConfig(t *testing.T) {
	cfg := &trabbits.HealthCheckConfig{
		Interval:           trabbits.Duration(30 * time.Second),
		Timeout:            trabbits.Duration(5 * time.Second),
		UnhealthyThreshold: 3,
		RecoveryInterval:   trabbits.Duration(60 * time.Second),
		Username:           "admin",
		Password:           "admin",
	}
	if cfg.Interval.ToDuration() != 30*time.Second {
		t.Errorf("Interval should be 30s, got %v", cfg.Interval.ToDuration())
	}
	if cfg.Timeout.ToDuration() != 5*time.Second {
		t.Errorf("Timeout should be 5s, got %v", cfg.Timeout.ToDuration())
	}
	if cfg.UnhealthyThreshold != 3 {
		t.Errorf("UnhealthyThreshold should be 3, got %d", cfg.UnhealthyThreshold)
	}
	if cfg.RecoveryInterval.ToDuration() != 60*time.Second {
		t.Errorf("RecoveryInterval should be 60s, got %v", cfg.RecoveryInterval.ToDuration())
	}
}

// Test upstream config with health check
func TestUpstreamConfigWithHealthCheck(t *testing.T) {
	upstream := trabbits.UpstreamConfig{
		Name: "test-cluster",
		Cluster: &trabbits.ClusterConfig{
			Nodes: []trabbits.NodeConfig{
				{Host: "localhost", Port: 5672},
				{Host: "localhost", Port: 5673},
			},
		},
		HealthCheck: &trabbits.HealthCheckConfig{
			Interval: trabbits.Duration(10 * time.Second),
			Username: "admin",
			Password: "admin",
		},
	}

	addrs := upstream.Addresses()
	if len(addrs) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(addrs))
	}

	if upstream.HealthCheck == nil {
		t.Error("HealthCheck should not be nil")
	}
}
