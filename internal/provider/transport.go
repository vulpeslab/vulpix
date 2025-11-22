package provider

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"github.com/vulpeslab/vulpix/internal/logger"
)

type LoggingRoundTripper struct {
	Proxied http.RoundTripper
}

func (lrt *LoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if debug logging is enabled
	if logger.Log.Enabled(context.Background(), slog.LevelDebug) {
		// Dump request
		reqDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			logger.Log.Error("Failed to dump request", "error", err)
		} else {
			logger.Log.Debug("HTTP Request", "dump", string(reqDump))
		}
	}

	resp, err := lrt.Proxied.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if logger.Log.Enabled(context.Background(), slog.LevelDebug) {
		// Dump response
		// Note: DumpResponse reads the body, so we need to be careful if it's large,
		// but for API calls it's usually fine.
		respDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			logger.Log.Error("Failed to dump response", "error", err)
		} else {
			logger.Log.Debug("HTTP Response", "dump", string(respDump))
		}
	}

	return resp, nil
}

func NewLoggingClient() *http.Client {
	return &http.Client{
		Transport: &LoggingRoundTripper{
			Proxied: http.DefaultTransport,
		},
	}
}
