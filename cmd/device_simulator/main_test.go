package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// TestExitCode_TagNotFoundIsNotFatal: a tag deleted while the Gateway runs is
// not an error of Run at all; IsNoPermission must not claim it.
func TestIsNoPermission(t *testing.T) {
	if devicelink.IsNoPermission(devicelink.ErrTagNotFound) {
		t.Error("a missing tag must not read as no permission")
	}
}
