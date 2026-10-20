package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Fake is a self-hosted stand-in for Google. /auth/fake-google/authorize shows
// a consent page where the user picks (or types) an account; it then redirects
// to the callback with a one-time code just like the real thing.
type Fake struct {
	baseURL     string
	redirectURL string
	mu          sync.Mutex
	codes       map[string]pendingCode
	Accounts    []Identity
}

type pendingCode struct {
	id      Identity
	expires time.Time
}

func NewFake(baseURL, redirectURL string) *Fake {
	return &Fake{
		baseURL:     strings.TrimRight(baseURL, "/"),
		redirectURL: redirectURL,
		codes:       map[string]pendingCode{},
		Accounts: []Identity{
			{Subject: "fake-google-1001", Email: "ada@acme.test", Name: "Ada Lovelace", AvatarURL: ""},
			{Subject: "fake-google-1002", Email: "grace@acme.test", Name: "Grace Hopper", AvatarURL: ""},
			{Subject: "fake-google-1003", Email: "linus@globex.test", Name: "Linus Torvalds", AvatarURL: ""},
		},
	}
}

func (f *Fake) AuthCodeURL(state string) string {
	return f.baseURL + "/auth/fake-google/authorize?state=" + url.QueryEscape(state)
}

func (f *Fake) Exchange(_ context.Context, code string) (*Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.codes[code]
	delete(f.codes, code)
	if !ok || time.Now().After(p.expires) {
		return nil, errors.New("fake google: invalid or expired code")
	}
	return &p.id, nil
}

func (f *Fake) Mount(r Router) {
	r.Get("/auth/fake-google/authorize", f.authorize)
	r.Post("/auth/fake-google/authorize", f.authorize)
}

func (f *Fake) authorize(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		email := strings.TrimSpace(strings.ToLower(r.Form.Get("email")))
		name := strings.TrimSpace(r.Form.Get("name"))
		if email == "" || !strings.Contains(email, "@") {
			http.Error(w, "email required", http.StatusBadRequest)
			return
		}
		id := Identity{Subject: "fake-google-" + hexOf(email), Email: email, Name: name}
		for _, a := range f.Accounts {
			if a.Email == email {
				id = a
				break
			}
		}
		if id.Name == "" {
			id.Name = strings.Split(email, "@")[0]
		}
		code := randomCode()
		f.mu.Lock()
		f.codes[code] = pendingCode{id: id, expires: time.Now().Add(5 * time.Minute)}
		f.mu.Unlock()
		http.Redirect(w, r, f.redirectURL+"?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(r.Form.Get("state")), http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>Fake Google Sign-in</title>
<style>body{font-family:system-ui,sans-serif;max-width:420px;margin:60px auto;padding:0 20px;color:#1f2937}
.card{border:1px solid #e5e7eb;border-radius:12px;padding:24px;margin-bottom:16px}
button{width:100%;text-align:left;padding:12px;border:1px solid #d1d5db;border-radius:8px;background:#fff;cursor:pointer;margin-bottom:8px;font-size:14px}
button:hover{background:#f3f4f6} input{width:100%;padding:10px;border:1px solid #d1d5db;border-radius:8px;margin:6px 0;box-sizing:border-box}
.badge{display:inline-block;background:#fef3c7;color:#92400e;padding:2px 8px;border-radius:999px;font-size:12px}</style></head><body>
<span class="badge">FAKE PROVIDER — development only</span>
<h2>Choose an account</h2><div class="card">`)
	for _, a := range f.Accounts {
		fmt.Fprintf(&b, `<form method="post" action="/auth/fake-google/authorize"><input type="hidden" name="state" value="%s"><input type="hidden" name="email" value="%s"><input type="hidden" name="name" value="%s"><button type="submit" data-testid="fake-google-account" data-email="%s"><strong>%s</strong><br><small>%s</small></button></form>`,
			html.EscapeString(state), html.EscapeString(a.Email), html.EscapeString(a.Name), html.EscapeString(a.Email), html.EscapeString(a.Name), html.EscapeString(a.Email))
	}
	fmt.Fprintf(&b, `</div><div class="card"><h3>Use another account</h3><form method="post" action="/auth/fake-google/authorize"><input type="hidden" name="state" value="%s"><input name="name" placeholder="Full name" data-testid="fake-google-name"><input name="email" placeholder="email@example.com" data-testid="fake-google-email"><button type="submit" data-testid="fake-google-continue">Continue</button></form></div></body></html>`, html.EscapeString(state))
	_, _ = w.Write([]byte(b.String()))
}

func randomCode() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hexOf derives a stable, collision-resistant subject from an email.
func hexOf(s string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(s)))
	return hex.EncodeToString(sum[:8])
}
