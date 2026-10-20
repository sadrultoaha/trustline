package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/org"
)

func (s *Server) getOrg(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	o, err := s.orgs.Get(r.Context(), s.pool, t.OrgID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, o)
}

func (s *Server) updateOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     *string             `json:"name"`
		Settings *domain.OrgSettings `json:"settings"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	t := TenantFrom(r.Context())
	o, err := s.orgs.Update(r.Context(), t.OrgID, org.UpdateOrg{Name: in.Name, Settings: in.Settings}, actorFrom(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, o)
}

// --- members -----------------------------------------------------------------

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	items, err := s.orgs.ListMembers(r.Context(), t.OrgID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) getMember(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.orgs.GetMember(r.Context(), t.OrgID, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Role domain.Role `json:"role"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.orgs.UpdateMemberRole(r.Context(), t.OrgID, id, in.Role, actorFrom(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if id == t.MembershipID {
		s.fail(w, r, domain.Invalid("member", "you cannot remove yourself; ask another admin"))
		return
	}
	if err := s.orgs.RemoveMember(r.Context(), t.OrgID, id, actorFrom(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- invites -----------------------------------------------------------------

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	items, err := s.orgs.ListInvites(r.Context(), t.OrgID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) inviteEmail(orgName, token string, role domain.Role) jobs.SendEmailArgs {
	link := s.cfg.BaseURL + "/invite/" + token
	html, text := mail.Render(mail.TemplateData{
		Title:       "You have been invited to " + orgName,
		Body:        "You were invited to join " + orgName + " on Trustline as " + string(role) + ". The invite expires in 7 days.",
		ActionURL:   link,
		ActionLabel: "Accept invitation",
		Footer:      "If you were not expecting this invitation, you can ignore this email.",
	})
	return jobs.SendEmailArgs{Subject: "Join " + orgName + " on Trustline", HTML: html, Text: text}
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	var in struct {
		Email string      `json:"email"`
		Role  domain.Role `json:"role"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	var inv *domain.Invite
	err := db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		o, err := s.orgs.Get(r.Context(), tx, t.OrgID)
		if err != nil {
			return err
		}
		i, token, err := s.orgs.CreateInvite(r.Context(), tx, t.OrgID, in.Email, in.Role, actorFrom(r))
		if err != nil {
			return err
		}
		inv = i
		m := s.inviteEmail(o.Name, token, i.Role)
		m.To = i.Email
		return s.jobs.EnqueueTx(r.Context(), tx, m)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, inv)
}

func (s *Server) resendInvite(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var inv *domain.Invite
	err = db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		old, err := s.orgs.GetInvite(r.Context(), t.OrgID, id)
		if err != nil {
			return err
		}
		if old.AcceptedAt != nil {
			return domain.ErrNotFound
		}
		o, err := s.orgs.Get(r.Context(), tx, t.OrgID)
		if err != nil {
			return err
		}
		// Re-creating rotates the token and extends expiry.
		i, token, err := s.orgs.CreateInvite(r.Context(), tx, t.OrgID, old.Email, old.Role, actorFrom(r))
		if err != nil {
			return err
		}
		inv = i
		m := s.inviteEmail(o.Name, token, i.Role)
		m.To = i.Email
		return s.jobs.EnqueueTx(r.Context(), tx, m)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, inv)
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.orgs.RevokeInvite(r.Context(), t.OrgID, id, actorFrom(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Public: preview and accept by token.

func (s *Server) invitePreview(w http.ResponseWriter, r *http.Request) {
	p, err := s.orgs.PreviewInvite(r.Context(), pathParam(r, "token"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, p)
}

// inviteAccept is for an already signed-in user; new users accept via signup.
func (s *Server) inviteAccept(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if u == nil {
		writeError(w, 401, "unauthorized", "sign in or sign up to accept this invite")
		return
	}
	sess := sessionFrom(r.Context())
	err := db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		m, err := s.orgs.AcceptInvite(r.Context(), tx, pathParam(r, "token"), u, clientIP(r))
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.Invalid("token", "you are already a member of this organization")
			}
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessions SET org_id = $2 WHERE id = $1`, sess.ID, m.OrgID); err != nil {
			return err
		}
		sess.OrgID = &m.OrgID
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp, err := s.buildMe(r, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, resp)
}

// --- activity ------------------------------------------------------------------

func (s *Server) listActivity(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := activity.ListFilter{TargetType: q.Get("target_type"), TargetID: q.Get("target_id"), Action: q.Get("action"), Cursor: q.Get("cursor"), Limit: limit}
	if a := q.Get("actor_id"); a != "" {
		id, err := domain.ParseID(a)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		f.ActorID = &id
	}
	page, err := activity.List(r.Context(), s.pool, t.OrgID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, page)
}
