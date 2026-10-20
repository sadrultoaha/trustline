package httpapi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/testutil"
)

// pendingEmailJobs reads send_email jobs straight from River's table so tests
// do not need a running worker.
func pendingEmailJobs(t *testing.T, app *testutil.App) []jobs.SendEmailArgs {
	t.Helper()
	rows, err := app.Pool.Query(t.Context(), `SELECT args FROM river_job WHERE kind = 'send_email' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []jobs.SendEmailArgs
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var a jobs.SendEmailArgs
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func inviteTokens(t *testing.T, app *testutil.App) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, j := range pendingEmailJobs(t, app) {
		if strings.Contains(j.Text, "/invite/") {
			out[j.To] = extractParam(j.Text, "/invite/")
		}
	}
	return out
}

// extractParam returns the token following marker up to whitespace or '&'.
func extractParam(text, marker string) string {
	i := strings.Index(text, marker)
	if i < 0 {
		return ""
	}
	rest := text[i+len(marker):]
	end := strings.IndexAny(rest, " \n\r&\"<")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
