package metrics

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/danlafeir/em/pkg/httputil"
)

// debugUpstreamResponseFlag is the flag name registered on commands that
// fetch from an upstream API. Deliberately not a persistent root flag — we
// don't want config commands or `em update` dumping bodies.
const debugUpstreamResponseFlag = "debug-upstream-response"

// registerUpstreamResponseFlag adds the --debug-upstream-response flag to a
// command. Wire the value through to each client via .WithDumpResponses().
func registerUpstreamResponseFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(debugUpstreamResponseFlag, false,
		"Print raw JSON response bodies from upstream API calls to stderr (may contain sensitive issue/vuln content)")
}

// dumpResponsesFlag reads --debug-upstream-response. Returns false if the
// flag is not registered on this command, so it's safe to call from any RunE.
func dumpResponsesFlag(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool(debugUpstreamResponseFlag)
	return v
}

// isHTTPStatus reports whether err (or anything it wraps) is an *httputil.APIError
// with the given status code. Used by config flows to detect specific failure
// modes without parsing error strings.
func isHTTPStatus(err error, status int) bool {
	var apiErr *httputil.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// surfaceUpstreamError unwraps an *httputil.APIError and returns it directly
// (so the user sees the actionable hint without a wrapper prefix). For other
// errors, it wraps with the given context. Use at the command-layer entry
// point where wrapping like "failed to connect to JIRA: <APIError>" only
// adds noise.
func surfaceUpstreamError(context string, err error) error {
	if httputil.IsAPIError(err) {
		return err
	}
	return wrapErr(context, err)
}

func wrapErr(context string, err error) error {
	if err == nil {
		return nil
	}
	return &wrappedErr{context: context, inner: err}
}

type wrappedErr struct {
	context string
	inner   error
}

func (w *wrappedErr) Error() string { return w.context + ": " + w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }
