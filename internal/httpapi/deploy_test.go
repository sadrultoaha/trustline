package httpapi_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/trustline/trustline/internal/blob"
	"github.com/trustline/trustline/internal/testutil"
)

// BLOB_MODE=postgres: a document uploaded through the API is stored in the
// database and the worker reads it back for the AI review.
func TestPostgresBlobStore(t *testing.T) {
	app := testutil.NewApp(t)
	app.Comp.Blob = &blob.Postgres{Pool: app.Pool}
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("pb")+"@example.test", testutil.Unique("Blob Co"))
	vid := c.Must(c.Post("/api/v1/vendors", obj{"name": "Blob Vendor", "data_access": "internal"}), 201).Body["id"].(string)
	pdf, err := os.ReadFile("../../testdata/vendors/northwind-soc2-type2.pdf")
	if err != nil {
		t.Fatal(err)
	}
	v := c.Must(c.Upload("/api/v1/vendors/"+vid+"/documents", "file", "report.pdf", pdf, map[string]string{"kind": "soc2_type2"}), 201).Body
	doc := v["documents_list"].([]any)[0].(obj)
	var n int
	if err := app.Pool.QueryRow(context.Background(), `SELECT count(*) FROM blobs WHERE length(data) = $1`, len(pdf)).Scan(&n); err != nil || n == 0 {
		t.Fatalf("blob row missing: n=%d err=%v", n, err)
	}
	testutil.Eventually(t, 30*time.Second, "assessment from postgres blob", func() bool {
		as := c.Get("/api/v1/vendors/" + vid).Body["assessments"].([]any)
		return len(as) > 0 && as[0].(obj)["status"] == "done"
	})
	r := c.Get("/api/v1/vendors/" + vid + "/documents/" + doc["id"].(string) + "/download")
	if r.Status != 200 || len(r.Raw) != len(pdf) {
		t.Fatalf("download: %d, %d bytes", r.Status, len(r.Raw))
	}
	if _, err := app.Comp.Blob.Get(context.Background(), "missing/key"); err != blob.ErrNotFound {
		t.Fatalf("missing key: %v", err)
	}
}

func TestAuthProviders(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.Client(t)
	if b := c.Must(c.Get("/api/v1/auth/providers"), 200).Body; b["google"] != true {
		t.Fatalf("providers: %v", b)
	}
}
