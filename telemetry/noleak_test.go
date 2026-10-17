package telemetry

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// leakRE finds a response that sends an error's text to the client. Internal
// errors (SQL, drivers, AWS) belong in Monitor: pass err as the cause —
// responder.SendError(w, "failed to …", status, err) or
// telemetry.RecordFailureW(w, msg, err) before http.Error(w, msg, status).
var leakRE = regexp.MustCompile(`(SendError|http\.Error)\(.*\.Error\(\)`)

// TestNoInternalErrorsInResponses scans this module's Go source. It lives in
// telemetry so it ships, identically, in every Hillview API.
func TestNoInternalErrorsInResponses(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if leakRE.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d sends an internal error to the client: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
