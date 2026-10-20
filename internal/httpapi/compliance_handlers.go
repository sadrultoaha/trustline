package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/rbac"
)

func (s *Server) mountCompliance(r chi.Router) {
	read := func(p rbac.Permission) func(http.Handler) http.Handler { return s.require(p) }
	r.With(read(rbac.ControlsRead)).Get("/dashboard", s.dashboard)

	r.With(read(rbac.ControlsRead)).Get("/frameworks", s.listFrameworks)
	r.With(read(rbac.ControlsRead)).Get("/frameworks/{id}", s.getFramework)
	r.With(read(rbac.ControlsWrite)).Post("/frameworks/{id}/adopt", s.adoptFramework)
	r.With(read(rbac.ControlsWrite)).Delete("/frameworks/{id}", s.removeFramework)

	r.With(read(rbac.ControlsRead)).Get("/controls", s.listControls)
	r.With(read(rbac.ControlsRead)).Get("/controls/{id}", s.getControl)
	r.With(read(rbac.ControlsWrite)).Patch("/controls/{id}", s.updateControl)
	r.With(read(rbac.EvidenceWrite)).Post("/controls/{id}/evidence", s.uploadEvidence)
	r.With(read(rbac.EvidenceRead)).Get("/evidence/{id}", s.getEvidence)
	r.With(read(rbac.EvidenceRead)).Get("/evidence/{id}/download", s.downloadEvidence)
	r.With(read(rbac.EvidenceWrite)).Post("/evidence/{id}/review", s.reviewEvidence)
	r.With(read(rbac.EvidenceWrite)).Delete("/evidence/{id}", s.deleteEvidence)

	r.With(read(rbac.IntegrationsRead)).Get("/integration-types", s.integrationTypes)
	r.With(read(rbac.IntegrationsRead)).Get("/integrations", s.listIntegrations)
	r.With(read(rbac.IntegrationsWrite)).Post("/integrations", s.connectIntegration)
	r.With(read(rbac.IntegrationsRead)).Get("/integrations/{id}", s.getIntegration)
	r.With(read(rbac.IntegrationsWrite)).Delete("/integrations/{id}", s.disconnectIntegration)
	r.With(read(rbac.IntegrationsWrite)).Post("/integrations/{id}/sync", s.syncIntegration)
	r.With(read(rbac.IntegrationsRead)).Get("/integrations/{id}/snapshot", s.integrationSnapshot)
	r.With(read(rbac.IntegrationsWrite)).Post("/integrations/{id}/sandbox/scenarios", s.setScenario)
	r.With(read(rbac.IntegrationsWrite)).Put("/integrations/{id}/sandbox/state", s.putSandboxState)

	r.With(read(rbac.TestsRead)).Get("/tests", s.listTests)
	r.With(read(rbac.TestsRead)).Get("/test-schema", s.testSchema)
	r.With(read(rbac.TestsWrite)).Post("/tests", s.createTest)
	r.With(read(rbac.TestsWrite)).Post("/tests/preview", s.previewTest)
	r.With(read(rbac.TestsRead)).Get("/tests/{id}", s.getTest)
	r.With(read(rbac.TestsWrite)).Patch("/tests/{id}", s.updateTest)
	r.With(read(rbac.TestsWrite)).Post("/tests/{id}/run", s.runTest)
	r.With(read(rbac.TestsRead)).Get("/test-runs/{id}", s.getRun)

	r.With(read(rbac.ControlsRead)).Get("/tasks", s.listTasks)
	r.With(read(rbac.ControlsRead)).Get("/tasks/{id}", s.getTask)
	r.With(read(rbac.ControlsWrite)).Patch("/tasks/{id}", s.updateTask)
	r.With(read(rbac.ControlsWrite)).Post("/tasks/{id}/regenerate", s.regenerateTask)
}

// withID parses {id} and calls fn, failing with 404 on a malformed id.
func (s *Server) withID(w http.ResponseWriter, r *http.Request, fn func(t Tenant, id domain.ID)) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	fn(TenantFrom(r.Context()), id)
}

func (s *Server) respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.comp.Dashboard(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, d, err)
}

// --- frameworks -------------------------------------------------------------

func (s *Server) listFrameworks(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListFrameworks(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) getFramework(w http.ResponseWriter, r *http.Request) {
	d, err := s.comp.GetFramework(r.Context(), TenantFrom(r.Context()).OrgID, chi.URLParam(r, "id"))
	s.respond(w, r, 200, d, err)
}

func (s *Server) adoptFramework(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := s.comp.AdoptFramework(r.Context(), t.OrgID, id, actorFrom(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.comp.GetFramework(r.Context(), t.OrgID, id)
	s.respond(w, r, 200, d, err)
}

func (s *Server) removeFramework(w http.ResponseWriter, r *http.Request) {
	err := s.comp.RemoveFramework(r.Context(), TenantFrom(r.Context()).OrgID, chi.URLParam(r, "id"), actorFrom(r))
	s.respond(w, r, 200, map[string]bool{"ok": true}, err)
}

// --- controls & evidence ------------------------------------------------------

func (s *Server) listControls(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := compliance.ControlFilter{FrameworkID: q.Get("framework"), Status: q.Get("status"), Query: q.Get("q")}
	if o := q.Get("owner"); o != "" {
		id, err := domain.ParseID(o)
		if err != nil {
			s.fail(w, r, domain.Invalid("owner", "invalid id"))
			return
		}
		f.OwnerID = &id
	}
	items, err := s.comp.ListControls(r.Context(), TenantFrom(r.Context()).OrgID, f)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) getControl(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetControl(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) updateControl(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			OwnerMembershipID *string `json:"owner_membership_id"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		var owner *domain.ID
		if in.OwnerMembershipID != nil && *in.OwnerMembershipID != "" {
			oid, err := domain.ParseID(*in.OwnerMembershipID)
			if err != nil {
				s.fail(w, r, domain.Invalid("owner_membership_id", "invalid id"))
				return
			}
			owner = &oid
		}
		if err := s.comp.SetOwner(r.Context(), t.OrgID, id, owner, actorFrom(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		d, err := s.comp.GetControl(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) uploadEvidence(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		r.Body = http.MaxBytesReader(w, r.Body, compliance.MaxEvidenceBytes+1<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			s.fail(w, r, domain.Invalid("file", "upload must be multipart/form-data and at most 20 MB"))
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			s.fail(w, r, domain.Invalid("file", "choose a file to upload"))
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, compliance.MaxEvidenceBytes+1))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		in := compliance.UploadInput{Title: r.FormValue("title"), Description: r.FormValue("description"), Filename: hdr.Filename, Data: data}
		if exp := r.FormValue("expires_at"); exp != "" {
			ts, err := time.Parse("2006-01-02", exp)
			if err != nil {
				s.fail(w, r, domain.Invalid("expires_at", "use YYYY-MM-DD"))
				return
			}
			in.ExpiresAt = &ts
		}
		ev, err := s.comp.UploadEvidence(r.Context(), t.OrgID, id, in, actorFrom(r))
		s.respond(w, r, 201, ev, err)
	})
}

func (s *Server) getEvidence(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		ev, err := s.comp.GetEvidence(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, ev, err)
	})
}

func (s *Server) downloadEvidence(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		ev, data, err := s.comp.EvidenceFile(r.Context(), t.OrgID, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", ev.ContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+ev.Filename+`"`)
		w.Header().Set("X-Content-SHA256", ev.SHA256)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
}

func (s *Server) reviewEvidence(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Verdict string `json:"verdict"`
			Reason  string `json:"reason"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		ev, err := s.comp.ReviewEvidence(r.Context(), t.OrgID, id, in.Verdict, in.Reason, actorFrom(r))
		s.respond(w, r, 200, ev, err)
	})
}

func (s *Server) deleteEvidence(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.DeleteEvidence(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

// --- integrations ---------------------------------------------------------------

func (s *Server) integrationTypes(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListTypes(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) listIntegrations(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListIntegrations(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) connectIntegration(w http.ResponseWriter, r *http.Request) {
	var in compliance.ConnectInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	it, err := s.comp.Connect(r.Context(), TenantFrom(r.Context()).OrgID, in, actorFrom(r))
	s.respond(w, r, 201, it, err)
}

func (s *Server) getIntegration(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		it, err := s.comp.GetIntegration(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, it, err)
	})
}

func (s *Server) disconnectIntegration(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.Disconnect(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) syncIntegration(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.RequestSync(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 202, map[string]bool{"queued": true}, err)
	})
}

func (s *Server) integrationSnapshot(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		snap, err := s.comp.LatestSnapshot(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, snap, err)
	})
}

func (s *Server) setScenario(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Key    string `json:"key"`
			Active bool   `json:"active"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		it, err := s.comp.SetScenario(r.Context(), t.OrgID, id, in.Key, in.Active, actorFrom(r))
		s.respond(w, r, 200, it, err)
	})
}

func (s *Server) putSandboxState(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Resources map[string][]map[string]any `json:"resources"`
		}
		if err := decodeLimit(r, &in, 5<<20); err != nil {
			s.fail(w, r, err)
			return
		}
		err := s.comp.PutSandboxState(r.Context(), t.OrgID, id, in.Resources, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

// --- tests -------------------------------------------------------------------

func (s *Server) listTests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := compliance.TestFilter{IntegrationType: q.Get("integration"), Status: q.Get("status"), Query: q.Get("q"), IncludeDisabled: q.Get("include_disabled") == "true"}
	if c := q.Get("control"); c != "" {
		id, err := domain.ParseID(c)
		if err != nil {
			s.fail(w, r, domain.Invalid("control", "invalid id"))
			return
		}
		f.ControlID = &id
	}
	items, err := s.comp.ListTests(r.Context(), TenantFrom(r.Context()).OrgID, f)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) testSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"connectors": s.comp.ResourceSchema(), "operators": operatorsInfo, "quantifiers": []string{"all", "any", "none", "count"}})
}

var operatorsInfo = []map[string]string{
	{"op": "eq", "label": "equals"}, {"op": "neq", "label": "does not equal"},
	{"op": "gt", "label": "greater than"}, {"op": "gte", "label": "at least"},
	{"op": "lt", "label": "less than"}, {"op": "lte", "label": "at most"},
	{"op": "in", "label": "is one of"}, {"op": "nin", "label": "is not one of"},
	{"op": "contains", "label": "contains"}, {"op": "not_contains", "label": "does not contain"},
	{"op": "exists", "label": "is set"}, {"op": "not_exists", "label": "is not set"},
	{"op": "matches", "label": "matches regex"}, {"op": "len_gt", "label": "length greater than"}, {"op": "len_lt", "label": "length less than"},
}

func (s *Server) createTest(w http.ResponseWriter, r *http.Request) {
	var in compliance.CustomTestInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.comp.CreateCustomTest(r.Context(), TenantFrom(r.Context()).OrgID, in, actorFrom(r))
	s.respond(w, r, 201, t, err)
}

func (s *Server) previewTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IntegrationType string          `json:"integration_type"`
		Rule            json.RawMessage `json:"rule"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.comp.PreviewRule(r.Context(), TenantFrom(r.Context()).OrgID, in.IntegrationType, in.Rule)
	s.respond(w, r, 200, res, err)
}

func (s *Server) getTest(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetTest(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) updateTest(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		if in.Enabled == nil {
			s.fail(w, r, domain.Invalid("enabled", "enabled is required"))
			return
		}
		row, err := s.comp.SetTestEnabled(r.Context(), t.OrgID, id, *in.Enabled, actorFrom(r))
		s.respond(w, r, 200, row, err)
	})
}

func (s *Server) runTest(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		row, err := s.comp.RunTestNow(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, row, err)
	})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetRun(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

// --- tasks ---------------------------------------------------------------------

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	t := TenantFrom(r.Context())
	f := compliance.TaskFilter{Status: q.Get("status")}
	if q.Get("mine") == "true" {
		f.AssigneeID = &t.MembershipID
	}
	if c := q.Get("control"); c != "" {
		id, err := domain.ParseID(c)
		if err != nil {
			s.fail(w, r, domain.Invalid("control", "invalid id"))
			return
		}
		f.ControlID = &id
	}
	items, err := s.comp.ListTasks(r.Context(), t.OrgID, f)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetTask(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Status               *string `json:"status"`
			Resolution           *string `json:"resolution"`
			AssigneeMembershipID *string `json:"assignee_membership_id"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		up := compliance.TaskUpdate{Status: in.Status, Resolution: in.Resolution}
		if in.AssigneeMembershipID != nil {
			if *in.AssigneeMembershipID == "" {
				up.ClearOwner = true
			} else {
				aid, err := domain.ParseID(*in.AssigneeMembershipID)
				if err != nil {
					s.fail(w, r, domain.Invalid("assignee_membership_id", "invalid id"))
					return
				}
				up.Assignee = &aid
			}
		}
		row, err := s.comp.UpdateTask(r.Context(), t.OrgID, id, up, actorFrom(r))
		s.respond(w, r, 200, row, err)
	})
}

func (s *Server) regenerateTask(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.RegenerateRemediation(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 202, map[string]bool{"queued": true}, err)
	})
}

// --- notifications ------------------------------------------------------------

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	if s.comp == nil {
		writeJSON(w, 200, map[string]any{"items": []any{}, "unread": 0})
		return
	}
	t := TenantFrom(r.Context())
	items, unread, err := s.comp.ListNotifications(r.Context(), t.OrgID, t.UserID, 30)
	s.respond(w, r, 200, map[string]any{"items": items, "unread": unread}, err)
}

func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	var ids []domain.ID
	for _, raw := range in.IDs {
		id, err := domain.ParseID(strings.TrimSpace(raw))
		if err != nil {
			s.fail(w, r, domain.Invalid("ids", "invalid id"))
			return
		}
		ids = append(ids, id)
	}
	t := TenantFrom(r.Context())
	err := s.comp.MarkNotificationsRead(r.Context(), t.OrgID, t.UserID, ids)
	s.respond(w, r, 200, map[string]bool{"ok": true}, err)
}

var _ = errors.New
