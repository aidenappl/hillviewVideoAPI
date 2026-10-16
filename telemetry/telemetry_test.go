package telemetry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	monitor "github.com/aidenappl/go-monitor"
	"github.com/gorilla/mux"
)

func TestMain(m *testing.M) {
	// The real rules (redaction included), with nothing shipped or printed.
	if err := monitor.Init(monitor.Config{Service: "telemetry-test", DisableStdout: true}); err != nil {
		panic(err)
	}
	m.Run()
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantStatus  int
		wantLevel   string
		wantMessage string
		wantError   string
		wantUser    string
	}{
		{
			name:       "success is info",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
			wantStatus: http.StatusOK,
			wantLevel:  monitor.LevelInfo,
		},
		{
			name: "recorded failure carries its cause",
			handler: func(w http.ResponseWriter, r *http.Request) {
				RecordFailureW(w, "failed to fetch videos", errors.New("db down"))
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantStatus:  http.StatusInternalServerError,
			wantLevel:   monitor.LevelError,
			wantMessage: "failed to fetch videos",
			wantError:   "db down",
		},
		{
			name: "http.Error body becomes the cause",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "sql: no rows", http.StatusInternalServerError)
			},
			wantStatus:  http.StatusInternalServerError,
			wantLevel:   monitor.LevelError,
			wantMessage: "sql: no rows",
		},
		{
			name:       "bad request is warn",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) },
			wantStatus: http.StatusBadRequest,
			wantLevel:  monitor.LevelWarn,
		},
		{
			name:       "unauthorized is info",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			wantStatus: http.StatusUnauthorized,
			wantLevel:  monitor.LevelInfo,
		},
		{
			name: "user id reaches the event",
			handler: func(w http.ResponseWriter, r *http.Request) {
				SetUserID(r.Context(), 42)
				w.WriteHeader(http.StatusOK)
			},
			wantStatus: http.StatusOK,
			wantLevel:  monitor.LevelInfo,
			wantUser:   "42",
		},
		{
			name:        "panic becomes a 500",
			handler:     func(w http.ResponseWriter, r *http.Request) { panic("boom") },
			wantStatus:  http.StatusInternalServerError,
			wantLevel:   monitor.LevelError,
			wantMessage: "panic",
			wantError:   "panic: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := monitor.StartRecording()
			defer rec.Stop()

			router := mux.NewRouter()
			api := router.PathPrefix("/core/v1.1").Subrouter()
			api.Use(Middleware)
			api.HandleFunc("/video/{id}", tt.handler)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/core/v1.1/video/7", nil))

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Error("X-Request-ID header not set")
			}

			events := rec.Named("http.request.end")
			if len(events) != 1 {
				t.Fatalf("got %d http.request.end events, want 1", len(events))
			}
			e := events[0]
			if e.Level != tt.wantLevel {
				t.Errorf("level = %q, want %q", e.Level, tt.wantLevel)
			}
			if e.RequestID == "" {
				t.Error("event has no request_id")
			}
			if e.UserID != tt.wantUser {
				t.Errorf("user_id = %q, want %q", e.UserID, tt.wantUser)
			}
			data, ok := e.Data.(map[string]any)
			if !ok {
				t.Fatalf("data is %T, want map", e.Data)
			}
			if data["path"] != "/core/v1.1/video/{id}" {
				t.Errorf("path = %v, want the route template", data["path"])
			}
			if data["request_path"] != "/core/v1.1/video/7" {
				t.Errorf("request_path = %v", data["request_path"])
			}
			if got, _ := data["error_message"].(string); got != tt.wantMessage {
				t.Errorf("error_message = %q, want %q", got, tt.wantMessage)
			}
			if got, _ := data["error"].(string); got != tt.wantError {
				t.Errorf("error = %q, want %q", got, tt.wantError)
			}
		})
	}
}

func TestMiddlewareReusesUpstreamRequestID(t *testing.T) {
	rec := monitor.StartRecording()
	defer rec.Stop()

	const upstream = "6f1c2b9e-3d4a-4c5b-8e7f-0a1b2c3d4e5f"
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-ID", upstream)
			next.ServeHTTP(w, r)
		})
	})
	api := router.PathPrefix("/api").Subrouter()
	api.Use(Middleware)
	api.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/ping", nil))

	events := rec.Named("http.request.end")
	if len(events) != 1 || events[0].RequestID != upstream {
		t.Fatalf("request_id = %v, want the upstream id %q", events, upstream)
	}
}
