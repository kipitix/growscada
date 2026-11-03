package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kipitix/gracedown"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/devicelink"
	"github.com/kipitix/growscada/internal/devicesim"
)

// netDown is an unreachable server.
type netDown struct{}

func (netDown) Error() string   { return "connection refused" }
func (netDown) Timeout() bool   { return false }
func (netDown) Temporary() bool { return true }

// TestExitCode checks the exit codes the task 34 asks of a Gateway: a refused
// token is no permission, an unusable local config is a configuration error,
// and everything else the protocol cannot settle is a protocol error.
func TestExitCode(t *testing.T) {
	status := func(code int) error {
		return fmt.Errorf("request: %w", &apiclient.StatusError{
			Method:  http.MethodGet,
			Problem: apiv0.Problem{Status: code},
		})
	}
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"revoked token", status(http.StatusUnauthorized), gracedown.ExitNoPerm},
		{"forbidden", status(http.StatusForbidden), gracedown.ExitNoPerm},
		{"bad request", status(http.StatusBadRequest), gracedown.ExitProtocol},
		{"gone", status(http.StatusGone), gracedown.ExitProtocol},
		{"bad url scheme", &url.Error{Op: "Get", URL: "localhost:9090", Err: errors.New("unsupported protocol scheme")}, gracedown.ExitProtocol},
		{"tls", &url.Error{Op: "Get", URL: "https://x", Err: errors.New("x509: certificate signed by unknown authority")}, gracedown.ExitProtocol},
		{"tag not supplied", fmt.Errorf("tag %q: %w", "a", devicesim.ErrConfig), gracedown.ExitConfig},
		{"missing environment", fmt.Errorf("%w: %q", devicesim.ErrEnvironmentNotFound, "nowhere"), gracedown.ExitConfig},
		{"unreachable", netDown{}, gracedown.ExitTempFail},
		{"context cancelled", context.Canceled, gracedown.ExitProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsNoPermission: a tag deleted while the Gateway runs is not an error of
// Run at all; IsNoPermission must not claim it.
func TestIsNoPermission(t *testing.T) {
	if devicelink.IsNoPermission(devicelink.ErrTagNotFound) {
		t.Error("a missing tag must not read as no permission")
	}
	if devicelink.IsUnauthenticated(&apiclient.StatusError{Problem: apiv0.Problem{Status: http.StatusForbidden}}) {
		t.Error("a refusal of the request must not read as a refused token")
	}
}

// TestReadConfig_FlagOverridesBeforeValidation: a secret may come from the
// environment alone, with no token in the file at all.
func TestReadConfig_FlagOverridesBeforeValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sim.yaml")
	err := os.WriteFile(path, []byte(`
environment: bench
tags:
  - tag: a
    pattern: {kind: constant, value: 1}
`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := readConfig(path, simulatorArgs{}); err == nil || !strings.Contains(err.Error(), "token (the Device Token) is required") {
		t.Errorf("readConfig without a token: got %v", err)
	}

	cfg, err := readConfig(path, simulatorArgs{Token: "gsc_dev_test", Server: "http://scada:9090", Control: ":9192"})
	if err != nil {
		t.Fatalf("readConfig with --token: %v", err)
	}
	if cfg.Token != "gsc_dev_test" || cfg.Server != "http://scada:9090" || cfg.Control != ":9192" {
		t.Errorf("overrides: got %+v", cfg)
	}
	if cfg.Environment != "bench" || len(cfg.Tags) != 1 {
		t.Errorf("config: got %+v", cfg)
	}
}
