// Package telemetry ships this service's events to Monitor's hillview zone.
//
// Monitor is never a boot requirement: with MONITOR_INGEST_URL unset the
// service runs exactly as before, and with Monitor unreachable events wait in
// the spool (MONITOR_SPOOL_DIR) until it answers.
//
// This file is identical in every Hillview Go API. Change it in all of them.
package telemetry

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	monitor "github.com/aidenappl/go-monitor"
	"github.com/gorilla/mux"
)

// ZONE is the Monitor zone every Hillview service reports into. go-monitor
// checks it against the ingest origin's /health at startup and complains if
// the URL points anywhere else.
const ZONE = "hillview"

// maxBodyCapture bounds how much of a 5xx response body is kept as its cause
// when the handler did not go through responder.SendError.
const maxBodyCapture = 512

// Config is read from the environment by each service's env package.
type Config struct {
	Service   string
	IngestURL string
	APIKey    string
	Env       string
	SpoolDir  string
	Stdout    bool
}

// Init starts the Monitor client. It never fails the boot: a bad config is
// logged and the service carries on without telemetry.
func Init(cfg Config) {
	if cfg.Env == "" {
		cfg.Env = "production"
	}
	if cfg.IngestURL == "" || cfg.APIKey == "" {
		log.Printf("telemetry: MONITOR_INGEST_URL or MONITOR_API_KEY is not set; events go nowhere")
	}
	err := monitor.Init(monitor.Config{
		Service:       cfg.Service,
		Env:           cfg.Env,
		IngestURL:     cfg.IngestURL,
		APIKey:        cfg.APIKey,
		Zone:          ZONE,
		SpoolDir:      cfg.SpoolDir,
		DisableStdout: !cfg.Stdout,
	})
	if err != nil {
		log.Printf("telemetry: monitor.Init failed, running without telemetry: %v", err)
		return
	}
	monitor.Info(context.Background(), "service.startup", map[string]any{
		"spool": cfg.SpoolDir != "",
	})
}

// Shutdown flushes buffered events. Call it last, after the HTTP server has
// drained, so the requests it finished are not lost.
func Shutdown() {
	monitor.Info(context.Background(), "service.shutdown", nil)
	monitor.Shutdown()
}

// Fatal reports a failure that is about to end the process and waits for it
// to ship. The caller still exits.
func Fatal(name string, err error, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["error"] = errString(err)
	data["error_type"] = fmt.Sprintf("%T", err)
	monitor.Fatal(context.Background(), name, data)
}

// Go runs fn on its own goroutine with panics reported instead of crashing
// the process. Use it for every background loop.
func Go(ctx context.Context, name string, fn func(ctx context.Context)) {
	monitor.Go(ctx, name, fn)
}

// Warn and Error emit a named event outside a request (background jobs).
func Warn(ctx context.Context, name string, err error, data map[string]any) {
	monitor.Warn(ctx, name, withErr(data, err))
}

func Error(ctx context.Context, name string, err error, data map[string]any) {
	monitor.Error(ctx, name, withErr(data, err))
}

func withErr(data map[string]any, err error) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	if err != nil {
		data["error"] = errString(err)
		data["error_type"] = fmt.Sprintf("%T", err)
	}
	return data
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// requestState is shared between the middleware and the code below it, which
// runs on a derived context the middleware cannot see.
type requestState struct {
	userID       string
	errorMessage string
	err          error
}

type stateKey struct{}

func stateFrom(ctx context.Context) *requestState {
	s, _ := ctx.Value(stateKey{}).(*requestState)
	return s
}

// SetUserID attributes the current request to a user. Call it wherever the
// access token has been resolved to a user.
func SetUserID(ctx context.Context, userID int) {
	if s := stateFrom(ctx); s != nil {
		s.userID = strconv.Itoa(userID)
	}
}

// RecordFailure attaches why a request failed to its http.request.end event.
// responder.SendError calls it, so a 5xx carries its cause and files a
// Monitor issue under the route instead of a bare status code.
func RecordFailure(r *http.Request, message string, err error) {
	if r == nil {
		return
	}
	if s := stateFrom(r.Context()); s != nil {
		s.errorMessage = message
		s.err = err
	}
}

// RecordFailureW is RecordFailure for code that only has the ResponseWriter.
func RecordFailureW(w http.ResponseWriter, message string, err error) {
	for w != nil {
		if rec, ok := w.(*recorder); ok {
			rec.state.errorMessage = message
			rec.state.err = err
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// Middleware emits one http.request.end event per request, recovers panics
// into a 500 and an error event, and propagates the request id.
//
// Register it with Router.Use on the API subrouter so the matched route
// template is known: Monitor groups issues by it.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Reuse an id an earlier middleware already put on the response, so
		// container logs and Monitor agree; otherwise mint one.
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" || !monitor.ValidCorrelationID(requestID) {
			requestID = monitor.NewRequestID()
			w.Header().Set("X-Request-ID", requestID)
		}
		state := &requestState{}
		ctx := monitor.WithRequestID(r.Context(), requestID)
		ctx = context.WithValue(ctx, stateKey{}, state)
		r = r.WithContext(ctx)

		rec := &recorder{ResponseWriter: w, status: http.StatusOK, state: state}

		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				monitor.ReportPanic(ctx, "http.panic", p)
				state.errorMessage = "panic"
				state.err = fmt.Errorf("panic: %v", p)
				if !rec.wroteHeader {
					rec.Header().Set("Content-Type", "application/json")
					rec.WriteHeader(http.StatusInternalServerError)
					_, _ = rec.Write([]byte(`{"error":null,"error_message":"internal server error","error_code":1000}`))
				}
			}
			emitRequest(ctx, r, rec, state, time.Since(start))
		}()

		next.ServeHTTP(rec, r)
	})
}

func emitRequest(ctx context.Context, r *http.Request, rec *recorder, state *requestState, elapsed time.Duration) {
	status := rec.status
	data := map[string]any{
		"method":       r.Method,
		"path":         routeTemplate(r),
		"request_path": r.URL.Path,
		"status_code":  status,
		"duration_ms":  elapsed.Milliseconds(),
		"client_ip":    clientIP(r),
		"user_agent":   r.UserAgent(),
	}
	if state.userID != "" {
		ctx = monitor.WithUserID(ctx, state.userID)
	}
	if state.errorMessage != "" {
		data["error_message"] = state.errorMessage
	}
	if state.err != nil {
		data["error"] = state.err.Error()
		data["error_type"] = fmt.Sprintf("%T", state.err)
		if root := rootCause(state.err); root != state.err {
			data["root_error_type"] = fmt.Sprintf("%T", root)
		}
	}
	if status >= 500 && state.errorMessage == "" && state.err == nil && len(rec.body) > 0 {
		// http.Error and hand-written responses: their body is the cause.
		data["error_message"] = strings.TrimSpace(string(rec.body))
	}

	switch {
	case status >= 500:
		monitor.Error(ctx, "http.request.end", data)
	case status == http.StatusUnauthorized, status == http.StatusNotFound, status == http.StatusMethodNotAllowed:
		monitor.Info(ctx, "http.request.end", data)
	case status >= 400:
		monitor.Warn(ctx, "http.request.end", data)
	default:
		monitor.Info(ctx, "http.request.end", data)
	}
}

func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}

func routeTemplate(r *http.Request) string {
	if route := mux.CurrentRoute(r); route != nil {
		if t, err := route.GetPathTemplate(); err == nil {
			return t
		}
	}
	return r.URL.Path
}

// clientIP trusts CF-Connecting-IP: every Hillview API sits behind Cloudflare
// and NPM, and NPM now only honours it from Cloudflare's ranges.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recorder captures the status code, and the start of a 5xx body as its
// cause, without changing what the client receives.
type recorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        []byte
	state       *requestState
}

func (w *recorder) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *recorder) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	if w.status >= 500 && len(w.body) < maxBodyCapture {
		n := maxBodyCapture - len(w.body)
		if n > len(b) {
			n = len(b)
		}
		w.body = append(w.body, b[:n]...)
	}
	return w.ResponseWriter.Write(b)
}

func (w *recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *recorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// ServeWithGracefulShutdown runs srv until SIGINT/SIGTERM, drains in-flight
// requests, then flushes telemetry. It returns when the process should exit.
func ServeWithGracefulShutdown(srv *http.Server, stop <-chan os.Signal) error {
	errc := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()

	select {
	case err := <-errc:
		if err != nil {
			Fatal("service.listen.failed", err, map[string]any{"addr": srv.Addr})
			return err
		}
		return nil
	case sig := <-stop:
		monitor.Info(context.Background(), "service.stopping", map[string]any{"signal": sig.String()})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			Warn(context.Background(), "service.shutdown.drain_incomplete", err, nil)
		}
		Shutdown()
		return nil
	}
}
