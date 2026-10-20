// Package mail sends transactional email. Real: SMTP (mailpit locally).
// Fake: in-memory outbox used by tests.
package mail

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTP sends through a plain (unauthenticated) SMTP relay such as mailpit.
type SMTP struct {
	Addr string
	From string
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", s.Addr, err)
	}
	host, _, _ := net.SplitHostPort(s.Addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	fromAddr := addrOnly(s.From)
	if err := c.Mail(fromAddr); err != nil {
		return err
	}
	if err := c.Rcpt(addrOnly(m.To)); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(build(s.From, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func addrOnly(s string) string {
	if i := strings.Index(s, "<"); i >= 0 {
		return strings.TrimSuffix(s[i+1:], ">")
	}
	return s
}

func build(from string, m Message) []byte {
	var b bytes.Buffer
	boundary := "tl-" + fmt.Sprint(time.Now().UnixNano())
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nDate: %s\r\n",
		from, m.To, mime.QEncoding.Encode("utf-8", m.Subject), time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, m.Text)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", boundary, m.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

// Memory is the fake sender; tests read Outbox.
type Memory struct {
	mu     sync.Mutex
	Outbox []Message
}

func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Outbox = append(m.Outbox, msg)
	return nil
}

func (m *Memory) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, len(m.Outbox))
	copy(out, m.Outbox)
	return out
}

// Templates ------------------------------------------------------------------

var layout = template.Must(template.New("layout").Parse(`<!doctype html><html><body style="font-family:system-ui,-apple-system,sans-serif;background:#f6f7f9;padding:32px">
<div style="max-width:520px;margin:0 auto;background:#fff;border-radius:12px;padding:32px;border:1px solid #e5e7eb">
<div style="font-weight:700;font-size:18px;color:#111827;margin-bottom:16px">Trustline</div>
<h2 style="font-size:18px;color:#111827;margin:0 0 12px">{{.Title}}</h2>
<p style="color:#374151;line-height:1.5">{{.Body}}</p>
{{if .ActionURL}}<p style="margin:24px 0"><a href="{{.ActionURL}}" style="background:#111827;color:#fff;padding:10px 18px;border-radius:8px;text-decoration:none;display:inline-block">{{.ActionLabel}}</a></p>
<p style="color:#6b7280;font-size:12px">Or copy this link: <a href="{{.ActionURL}}">{{.ActionURL}}</a></p>{{end}}
<p style="color:#9ca3af;font-size:12px;margin-top:24px">{{.Footer}}</p>
</div></body></html>`))

type TemplateData struct {
	Title       string
	Body        string
	ActionURL   string
	ActionLabel string
	Footer      string
}

// Render produces HTML + text bodies from the shared layout.
func Render(d TemplateData) (html, text string) {
	var b bytes.Buffer
	_ = layout.Execute(&b, d)
	t := d.Title + "\n\n" + d.Body + "\n"
	if d.ActionURL != "" {
		t += "\n" + d.ActionLabel + ": " + d.ActionURL + "\n"
	}
	t += "\n" + d.Footer + "\n"
	return b.String(), t
}
