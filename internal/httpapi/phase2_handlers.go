package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/rbac"
)

func (s *Server) mountPhase2(r chi.Router) {
	p := func(perm rbac.Permission) func(http.Handler) http.Handler { return s.require(perm) }
	// Policies
	r.With(p(rbac.PoliciesRead)).Get("/policies", s.listPolicies)
	r.With(p(rbac.PoliciesWrite)).Get("/policy-library", s.policyLibrary)
	r.With(p(rbac.PoliciesWrite)).Post("/policies", s.createPolicy)
	r.With(p(rbac.PoliciesRead)).Get("/policies/{id}", s.getPolicy)
	r.With(p(rbac.PoliciesWrite)).Patch("/policies/{id}", s.updatePolicy)
	r.With(p(rbac.PoliciesWrite)).Post("/controls/{id}/policies", s.linkControlPolicy)
	r.With(p(rbac.PoliciesWrite)).Post("/policies/{id}/versions", s.newPolicyVersion)
	r.With(p(rbac.PoliciesWrite)).Post("/policies/{id}/reviewed", s.markPolicyReviewed)
	r.With(p(rbac.PoliciesWrite)).Put("/policy-versions/{id}", s.saveDraft)
	r.With(p(rbac.PoliciesWrite)).Post("/policy-versions/{id}/submit", s.submitVersion)
	r.With(p(rbac.PoliciesWrite)).Post("/policy-versions/{id}/decide", s.decideVersion)
	r.With(p(rbac.PoliciesWrite)).Post("/policy-versions/{id}/publish", s.publishVersion)
	r.With(p(rbac.PoliciesWrite), p(rbac.AIUse)).Post("/policy-versions/{id}/ai-draft", s.aiDraftVersion)
	r.With(p(rbac.PoliciesWrite), p(rbac.AIUse)).Post("/policy-versions/{id}/consistency", s.checkConsistency)
	r.With(p(rbac.PoliciesAccept)).Post("/policy-versions/{id}/acknowledge", s.acknowledgeVersion)

	// People, devices, training
	r.With(p(rbac.PeopleRead)).Get("/people", s.listPeople)
	r.With(p(rbac.PeopleRead)).Get("/people/{id}", s.getPerson)
	r.With(p(rbac.PeopleWrite)).Post("/people/{id}/background-check", s.attestBackground)
	r.With(p(rbac.PeopleWrite)).Post("/tasks/{id}/complete", s.completeTask)
	r.With(p(rbac.PeopleRead)).Get("/devices", s.listDevices)
	r.With(p(rbac.PeopleWrite)).Delete("/devices/{id}", s.deleteDevice)
	r.With(p(rbac.OrgWrite)).Get("/device-token", s.getDeviceToken)
	r.With(p(rbac.OrgWrite)).Post("/device-token", s.rotateDeviceToken)
	r.With(p(rbac.TasksOwn)).Get("/training", s.getTraining)
	r.With(p(rbac.TasksOwn)).Post("/training/submit", s.submitTraining)
	r.With(p(rbac.TasksOwn)).Get("/me/work", s.myWork)

	// Access reviews
	r.With(p(rbac.ControlsRead)).Get("/access-reviews", s.listCampaigns)
	r.With(p(rbac.ControlsWrite)).Post("/access-reviews", s.createCampaign)
	r.With(p(rbac.ControlsRead)).Get("/access-reviews/{id}", s.getCampaign)
	r.With(p(rbac.ControlsRead)).Get("/access-reviews/{id}/export.csv", s.exportCampaign)
	r.With(p(rbac.ControlsWrite)).Post("/access-reviews/{id}/complete", s.completeCampaign)
	r.With(p(rbac.TasksOwn)).Post("/access-review-items/{id}/decision", s.decideItem)
}

// --- policies -------------------------------------------------------------------

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	items, err := s.comp.ListPolicies(r.Context(), t.OrgID, !rbac.Can(t.Role, rbac.PoliciesWrite) && t.Role != domain.RoleAuditor)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) policyLibrary(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.PolicyLibrary(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TemplateID string `json:"template_id"`
		Title      string `json:"title"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.comp.CreatePolicy(r.Context(), TenantFrom(r.Context()).OrgID, in.TemplateID, in.Title, actorFrom(r))
	s.respond(w, r, 201, d, err)
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		full := rbac.Can(t.Role, rbac.PoliciesWrite) || t.Role == domain.RoleAuditor
		d, err := s.comp.GetPolicy(r.Context(), t.OrgID, id, full)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) updatePolicy(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			OwnerMembershipID *string   `json:"owner_membership_id"`
			ControlCodes      *[]string `json:"control_codes"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		if in.OwnerMembershipID != nil {
			var owner *domain.ID
			if *in.OwnerMembershipID != "" {
				oid, err := domain.ParseID(*in.OwnerMembershipID)
				if err != nil {
					s.fail(w, r, domain.Invalid("owner_membership_id", "invalid id"))
					return
				}
				owner = &oid
			}
			if err := s.comp.SetPolicyOwner(r.Context(), t.OrgID, id, owner, actorFrom(r)); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		if in.ControlCodes != nil {
			if err := s.comp.SetPolicyControls(r.Context(), t.OrgID, id, *in.ControlCodes, actorFrom(r)); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		d, err := s.comp.GetPolicy(r.Context(), t.OrgID, id, true)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) newPolicyVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		v, err := s.comp.NewVersion(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 201, v, err)
	})
}

func (s *Server) markPolicyReviewed(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.MarkReviewed(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) saveDraft(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			ContentHTML   string `json:"content_html"`
			ChangeSummary string `json:"change_summary"`
		}
		if err := decodeLimit(r, &in, 3<<20); err != nil {
			s.fail(w, r, err)
			return
		}
		err := s.comp.UpdateDraft(r.Context(), t.OrgID, id, in.ContentHTML, in.ChangeSummary, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) submitVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Approver string `json:"approver_membership_id"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		aid, err := domain.ParseID(in.Approver)
		if err != nil {
			s.fail(w, r, domain.Invalid("approver_membership_id", "choose an approver"))
			return
		}
		err = s.comp.SubmitForApproval(r.Context(), t.OrgID, id, aid, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) decideVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Approve bool   `json:"approve"`
			Comment string `json:"comment"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		err := s.comp.DecideVersion(r.Context(), t.OrgID, id, in.Approve, in.Comment, actorFrom(r), t.Role, t.MembershipID)
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) publishVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.Publish(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) aiDraftVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Instructions string `json:"instructions"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.comp.AIDraftVersion(r.Context(), s.comp.AI, t.OrgID, id, in.Instructions, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) checkConsistency(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		rep, err := s.comp.CheckConsistency(r.Context(), s.comp.AI, t.OrgID, id)
		s.respond(w, r, 200, rep, err)
	})
}

func (s *Server) acknowledgeVersion(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.Acknowledge(r.Context(), t.OrgID, id, t.UserID, clientIP(r), actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

// --- people / devices / training -------------------------------------------------

func (s *Server) listPeople(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListPeople(r.Context(), TenantFrom(r.Context()).OrgID, r.URL.Query().Get("status"))
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetPerson(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) attestBackground(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Status string `json:"status"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.comp.AttestBackgroundCheck(r.Context(), t.OrgID, id, in.Status, actorFrom(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		d, err := s.comp.GetPerson(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) completeTask(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Note string `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		err := s.comp.CompleteManualItem(r.Context(), t.OrgID, id, strings.TrimSpace(in.Note), actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListDevices(r.Context(), TenantFrom(r.Context()).OrgID, nil)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.DeleteDevice(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) getDeviceToken(w http.ResponseWriter, r *http.Request) {
	info, err := s.comp.DeviceToken(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, info, err)
}

func (s *Server) rotateDeviceToken(w http.ResponseWriter, r *http.Request) {
	info, err := s.comp.RotateDeviceToken(r.Context(), TenantFrom(r.Context()).OrgID, actorFrom(r))
	s.respond(w, r, 201, info, err)
}

// agentReport is called by the device agent with the org's enrollment token.
func (s *Server) agentReport(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(token, "tld_") {
		writeError(w, 401, "unauthorized", "missing device enrollment token")
		return
	}
	if s.limited(w, r, "agent:"+clientIP(r), 600, 10*time.Minute) {
		return
	}
	orgID, err := s.comp.OrgForDeviceToken(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in compliance.DeviceReport
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.comp.IngestDeviceReport(r.Context(), orgID, in)
	s.respond(w, r, 200, d, err)
}

func (s *Server) getTraining(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	st, err := s.comp.TrainingFor(r.Context(), t.OrgID, t.UserID)
	s.respond(w, r, 200, st, err)
}

func (s *Server) submitTraining(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Answers map[string]int `json:"answers"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	t := TenantFrom(r.Context())
	res, err := s.comp.SubmitQuiz(r.Context(), t.OrgID, t.UserID, in.Answers, actorFrom(r))
	s.respond(w, r, 200, res, err)
}

func (s *Server) myWork(w http.ResponseWriter, r *http.Request) {
	t := TenantFrom(r.Context())
	u := userFrom(r.Context())
	wk, err := s.comp.MyWork(r.Context(), t.OrgID, t.UserID, t.MembershipID, u.Email)
	s.respond(w, r, 200, wk, err)
}

// --- access reviews ---------------------------------------------------------------

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListCampaigns(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	var in compliance.CampaignInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.comp.CreateCampaign(r.Context(), TenantFrom(r.Context()).OrgID, in, actorFrom(r))
	s.respond(w, r, 201, d, err)
}

func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.GetCampaign(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) exportCampaign(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		data, name, err := s.comp.ExportCampaign(r.Context(), t.OrgID, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		_, _ = w.Write(data)
	})
}

func (s *Server) completeCampaign(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		d, err := s.comp.CompleteCampaign(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, d, err)
	})
}

func (s *Server) decideItem(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Decision string `json:"decision"`
			Comment  string `json:"comment"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		it, err := s.comp.Decide(r.Context(), t.OrgID, id, in.Decision, in.Comment, actorFrom(r), t.Role, t.MembershipID)
		s.respond(w, r, 200, it, err)
	})
}

func (s *Server) linkControlPolicy(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			PolicyID string `json:"policy_id"`
			Linked   bool   `json:"linked"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		pid, err := domain.ParseID(in.PolicyID)
		if err != nil {
			s.fail(w, r, domain.Invalid("policy_id", "choose a policy"))
			return
		}
		if err := s.comp.LinkPolicyToControl(r.Context(), t.OrgID, id, pid, in.Linked, actorFrom(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		d, err := s.comp.GetControl(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, d, err)
	})
}
