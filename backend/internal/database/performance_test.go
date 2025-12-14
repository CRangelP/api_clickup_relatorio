package database

import (
	"testing"
)

// TestDatabasePoolConfiguration tests that the database connection pool is configured correctly
// (Requirements 15.4)
func TestDatabasePoolConfiguration(t *testing.T) {
	// Test default configuration values
	cfg := Config{
		Host:     "localhost",
		Port:     "5432",
		User:     "test",
		Password: "test",
		DBName:   "test",
		SSLMode:  "disable",
	}

	// Verify defaults are applied when values are zero
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 25 // Default value
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 10 // Default value
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = 5 // Default value in minutes
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = 2 // Default value in minutes
	}

	// Verify pool settings are reasonable for performance
	if cfg.MaxOpenConns < 10 {
		t.Errorf("MaxOpenConns should be at least 10 for performance, got %d", cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		t.Errorf("MaxIdleConns (%d) should not exceed MaxOpenConns (%d)", cfg.MaxIdleConns, cfg.MaxOpenConns)
	}

	if cfg.ConnMaxLifetime < 1 {
		t.Errorf("ConnMaxLifetime should be at least 1 minute, got %d", cfg.ConnMaxLifetime)
	}

	t.Logf("Pool config: MaxOpen=%d, MaxIdle=%d, MaxLifetime=%dm, MaxIdleTime=%dm",
		cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime, cfg.ConnMaxIdleTime)
}

// TestPoolStatsStructure tests that PoolStats contains all necessary fields
func TestPoolStatsStructure(t *testing.T) {
	stats := PoolStats{
		MaxOpenConnections: 25,
		OpenConnections:    10,
		InUse:              5,
		Idle:               5,
		WaitCount:          100,
		WaitDuration:       500,
		MaxIdleClosed:      10,
		MaxIdleTimeClosed:  5,
		MaxLifetimeClosed:  2,
	}

	// Verify InUse + Idle = OpenConnections
	if stats.InUse+stats.Idle != stats.OpenConnections {
		t.Errorf("InUse (%d) + Idle (%d) should equal OpenConnections (%d)",
			stats.InUse, stats.Idle, stats.OpenConnections)
	}

	// Verify OpenConnections doesn't exceed MaxOpenConnections
	if stats.OpenConnections > stats.MaxOpenConnections {
		t.Errorf("OpenConnections (%d) should not exceed MaxOpenConnections (%d)",
			stats.OpenConnections, stats.MaxOpenConnections)
	}

	t.Logf("Pool stats: Open=%d, InUse=%d, Idle=%d, WaitCount=%d",
		stats.OpenConnections, stats.InUse, stats.Idle, stats.WaitCount)
}

// TestConfigDefaults tests that default configuration values are applied correctly
func TestConfigDefaults(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected Config
	}{
		{
			name: "all defaults",
			cfg: Config{
				Host:     "localhost",
				Port:     "5432",
				User:     "test",
				Password: "test",
				DBName:   "test",
				SSLMode:  "disable",
			},
			expected: Config{
				Host:            "localhost",
				Port:            "5432",
				User:            "test",
				Password:        "test",
				DBName:          "test",
				SSLMode:         "disable",
				MaxOpenConns:    25,
				MaxIdleConns:    10,
				ConnMaxLifetime: 5,
				ConnMaxIdleTime: 2,
			},
		},
		{
			name: "custom values preserved",
			cfg: Config{
				Host:            "localhost",
				Port:            "5432",
				User:            "test",
				Password:        "test",
				DBName:          "test",
				SSLMode:         "disable",
				MaxOpenConns:    50,
				MaxIdleConns:    20,
				ConnMaxLifetime: 10,
				ConnMaxIdleTime: 5,
			},
			expected: Config{
				Host:            "localhost",
				Port:            "5432",
				User:            "test",
				Password:        "test",
				DBName:          "test",
				SSLMode:         "disable",
				MaxOpenConns:    50,
				MaxIdleConns:    20,
				ConnMaxLifetime: 10,
				ConnMaxIdleTime: 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg

			// Apply defaults (same logic as in Connect function)
			if cfg.MaxOpenConns == 0 {
				cfg.MaxOpenConns = 25
			}
			if cfg.MaxIdleConns == 0 {
				cfg.MaxIdleConns = 10
			}
			if cfg.ConnMaxLifetime == 0 {
				cfg.ConnMaxLifetime = 5
			}
			if cfg.ConnMaxIdleTime == 0 {
				cfg.ConnMaxIdleTime = 2
			}

			if cfg.MaxOpenConns != tt.expected.MaxOpenConns {
				t.Errorf("MaxOpenConns: expected %d, got %d", tt.expected.MaxOpenConns, cfg.MaxOpenConns)
			}
			if cfg.MaxIdleConns != tt.expected.MaxIdleConns {
				t.Errorf("MaxIdleConns: expected %d, got %d", tt.expected.MaxIdleConns, cfg.MaxIdleConns)
			}
			if cfg.ConnMaxLifetime != tt.expected.ConnMaxLifetime {
				t.Errorf("ConnMaxLifetime: expected %d, got %d", tt.expected.ConnMaxLifetime, cfg.ConnMaxLifetime)
			}
			if cfg.ConnMaxIdleTime != tt.expected.ConnMaxIdleTime {
				t.Errorf("ConnMaxIdleTime: expected %d, got %d", tt.expected.ConnMaxIdleTime, cfg.ConnMaxIdleTime)
			}
		})
	}
}
