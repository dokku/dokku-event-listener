package commands

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestShouldRebuildOnDie(t *testing.T) {
	tests := []struct {
		name         string
		policy       container.RestartPolicyMode
		restartCount int
		maxRetry     int
		want         bool
	}{
		{
			name:         "always never rebuilds (reported bug)",
			policy:       container.RestartPolicyAlways,
			restartCount: 0,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "always never rebuilds with restart count",
			policy:       container.RestartPolicyAlways,
			restartCount: 5,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "unless-stopped never rebuilds",
			policy:       container.RestartPolicyUnlessStopped,
			restartCount: 0,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "no policy never rebuilds",
			policy:       container.RestartPolicyDisabled,
			restartCount: 0,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "empty policy never rebuilds",
			policy:       "",
			restartCount: 0,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "on-failure with unlimited retries never rebuilds",
			policy:       container.RestartPolicyOnFailure,
			restartCount: 0,
			maxRetry:     0,
			want:         false,
		},
		{
			name:         "on-failure not yet exhausted does not rebuild",
			policy:       container.RestartPolicyOnFailure,
			restartCount: 5,
			maxRetry:     10,
			want:         false,
		},
		{
			name:         "on-failure one short does not rebuild",
			policy:       container.RestartPolicyOnFailure,
			restartCount: 9,
			maxRetry:     10,
			want:         false,
		},
		{
			name:         "on-failure exhausted rebuilds",
			policy:       container.RestartPolicyOnFailure,
			restartCount: 10,
			maxRetry:     10,
			want:         true,
		},
		{
			name:         "on-failure exceeded rebuilds",
			policy:       container.RestartPolicyOnFailure,
			restartCount: 11,
			maxRetry:     10,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restartPolicy := container.RestartPolicy{
				Name:              tt.policy,
				MaximumRetryCount: tt.maxRetry,
			}
			if got := shouldRebuildOnDie(restartPolicy, tt.restartCount); got != tt.want {
				t.Errorf("shouldRebuildOnDie(%+v, %d) = %v, want %v", restartPolicy, tt.restartCount, got, tt.want)
			}
		})
	}
}
