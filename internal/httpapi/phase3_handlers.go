package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/rbac"
)

func (s *Server) mountPhase3(r chi.Router) {
	p := func(perm rbac.Permission) func(http.Handler) http.Handler { return s.require(perm) }
	r.With(p(rbac.RisksRead)).Get("/risks", s.listRisks)
	r.With(p(rbac.RisksRead)).Get("/risks/heatmap", s.riskHeatmap)
	r.With(p(rbac.RisksRead)).Get("/risk-assessments", s.listRiskAssessments)
	r.With(p(rbac.RisksWrite)).Post("/risk-assessments", s.recordRiskAssessment)
	r.With(p(rbac.RisksWrite), p(rbac.AIUse)).Post("/risks/ai-assessment", s.aiRiskAssessment)
	r.With(p(rbac.RisksWrite)).Post("/risks", s.createRisk)
	r.With(p(rbac.RisksRead)).Get("/risks/{id}", s.getRisk)
	r.With(p(rbac.RisksWrite)).Patch("/risks/{id}", s.updateRisk)
	r.With(p(rbac.RisksWrite)).Post("/risks/{id}/accept", s.acceptRisk)
	r.With(p(rbac.RisksWrite)).Post("/risks/{id}/dismiss", s.dismissRisk)
	r.With(p(rbac.RisksWrite)).Post("/risks/{id}/reviewed", s.reviewRisk)

	r.With(p(rbac.VendorsRead)).Get("/vendors", s.listVendors)
	r.With(p(rbac.VendorsWrite)).Post("/vendors", s.createVendor)
	r.With(p(rbac.VendorsRead)).Get("/vendors/{id}", s.getVendor)
	r.With(p(rbac.VendorsWrite)).Patch("/vendors/{id}", s.updateVendor)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/triage", s.triageVendor)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/restore", s.restoreVendor)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/documents", s.uploadVendorDocument)
	r.With(p(rbac.VendorsRead)).Get("/vendors/{id}/documents/{doc}/download", s.downloadVendorDocument)
	r.With(p(rbac.VendorsWrite), p(rbac.AIUse)).Post("/vendors/{id}/documents/{doc}/assess", s.assessVendorDocument)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/questionnaires", s.sendQuestionnaire)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/apply-tier", s.applyVendorTier)
	r.With(p(rbac.VendorsWrite)).Post("/vendors/{id}/reviewed", s.reviewVendor)
}

// mountVendorPortal registers the public, token-authenticated portal.
func (s *Server) mountVendorPortal(r chi.Router) {
	limit := s.rateLimit("portal", 300, 10*time.Minute, clientIP)
	r.With(limit).Get("/vendor-portal/{token}", s.portalGet)
	r.With(limit).Put("/vendor-portal/{token}", s.portalSave)
}

// --- risks --------------------------------------------------------------------

func (s *Server) listRisks(w http.ResponseWriter, r *http.Request) {
	f := compliance.RiskFilter{Status: r.URL.Query().Get("status")}
	if c := r.URL.Query().Get("control"); c != "" {
		id, err := domain.ParseID(c)
		if err != nil {
			s.fail(w, r, domain.Invalid("control", "invalid id"))
			return
		}
		f.ControlID = &id
	}
	items, err := s.comp.ListRisks(r.Context(), TenantFrom(r.Context()).OrgID, f)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) riskHeatmap(w http.ResponseWriter, r *http.Request) {
	h, err := s.comp.RiskHeatmap(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, h, err)
}

func (s *Server) listRiskAssessments(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListRiskAssessments(r.Context(), TenantFrom(r.Context()).OrgID)
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) recordRiskAssessment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Summary string `json:"summary"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Summary == "" {
		s.fail(w, r, domain.Invalid("summary", "describe the assessment"))
		return
	}
	err := s.comp.RecordManualAssessment(r.Context(), TenantFrom(r.Context()).OrgID, in.Summary, actorFrom(r))
	s.respond(w, r, 201, map[string]bool{"ok": true}, err)
}

func (s *Server) aiRiskAssessment(w http.ResponseWriter, r *http.Request) {
	items, summary, err := s.comp.CompleteWithAI(r.Context(), s.comp.AI, TenantFrom(r.Context()).OrgID, actorFrom(r))
	if items == nil {
		items = []compliance.Risk{}
	}
	s.respond(w, r, 200, map[string]any{"items": items, "summary": summary}, err)
}

func (s *Server) createRisk(w http.ResponseWriter, r *http.Request) {
	var in compliance.RiskInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	risk, err := s.comp.CreateRisk(r.Context(), TenantFrom(r.Context()).OrgID, in, actorFrom(r))
	s.respond(w, r, 201, risk, err)
}

func (s *Server) getRisk(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		risk, err := s.comp.GetRisk(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, risk, err)
	})
}

func (s *Server) updateRisk(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in compliance.RiskInput
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		risk, err := s.comp.UpdateRisk(r.Context(), t.OrgID, id, in, actorFrom(r))
		s.respond(w, r, 200, risk, err)
	})
}

func (s *Server) acceptRisk(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		risk, err := s.comp.AcceptSuggestion(r.Context(), t.OrgID, id, actorFrom(r), t.MembershipID)
		s.respond(w, r, 200, risk, err)
	})
}

func (s *Server) dismissRisk(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		err := s.comp.DismissSuggestion(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, map[string]bool{"ok": true}, err)
	})
}

func (s *Server) reviewRisk(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		risk, err := s.comp.MarkRiskReviewed(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, risk, err)
	})
}

// --- vendors -------------------------------------------------------------------

func (s *Server) listVendors(w http.ResponseWriter, r *http.Request) {
	items, err := s.comp.ListVendors(r.Context(), TenantFrom(r.Context()).OrgID, r.URL.Query().Get("status"))
	s.respond(w, r, 200, map[string]any{"items": items}, err)
}

func (s *Server) createVendor(w http.ResponseWriter, r *http.Request) {
	var in compliance.VendorInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.comp.CreateVendor(r.Context(), TenantFrom(r.Context()).OrgID, in, actorFrom(r))
	s.respond(w, r, 201, v, err)
}

func (s *Server) getVendor(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		v, err := s.comp.GetVendor(r.Context(), t.OrgID, id)
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) updateVendor(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in compliance.VendorInput
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.comp.UpdateVendor(r.Context(), t.OrgID, id, in, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) triageVendor(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			Add bool `json:"add"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.comp.TriageVendor(r.Context(), t.OrgID, id, in.Add, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) restoreVendor(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		v, err := s.comp.RestoreVendor(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) uploadVendorDocument(w http.ResponseWriter, r *http.Request) {
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
		in := compliance.VendorDocInput{Kind: r.FormValue("kind"), Title: r.FormValue("title"), Filename: hdr.Filename, Data: data}
		if exp := r.FormValue("expires_at"); exp != "" {
			ts, err := time.Parse("2006-01-02", exp)
			if err != nil {
				s.fail(w, r, domain.Invalid("expires_at", "use YYYY-MM-DD"))
				return
			}
			in.ExpiresAt = &ts
		}
		v, err := s.comp.UploadVendorDocument(r.Context(), t.OrgID, id, in, actorFrom(r))
		s.respond(w, r, 201, v, err)
	})
}

func (s *Server) downloadVendorDocument(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		docID, err := pathID(r, "doc")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		d, data, err := s.comp.VendorDocumentFile(r.Context(), t.OrgID, id, docID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", d.ContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+d.Filename+`"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
}

func (s *Server) assessVendorDocument(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		docID, err := pathID(r, "doc")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		err = s.comp.RequestAssessment(r.Context(), t.OrgID, id, docID, actorFrom(r))
		s.respond(w, r, 202, map[string]bool{"queued": true}, err)
	})
}

func (s *Server) sendQuestionnaire(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		var in struct {
			RecipientEmail string `json:"recipient_email"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		q, err := s.comp.SendQuestionnaire(r.Context(), t.OrgID, id, in.RecipientEmail, actorFrom(r))
		s.respond(w, r, 201, q, err)
	})
}

func (s *Server) applyVendorTier(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		v, err := s.comp.ApplyRecommendedTier(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) reviewVendor(w http.ResponseWriter, r *http.Request) {
	s.withID(w, r, func(t Tenant, id domain.ID) {
		v, err := s.comp.MarkVendorReviewed(r.Context(), t.OrgID, id, actorFrom(r))
		s.respond(w, r, 200, v, err)
	})
}

func (s *Server) portalGet(w http.ResponseWriter, r *http.Request) {
	v, err := s.comp.PortalGet(r.Context(), chi.URLParam(r, "token"))
	s.respond(w, r, 200, v, err)
}

func (s *Server) portalSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Answers map[string]string `json:"answers"`
		Submit  bool              `json:"submit"`
	}
	if err := decodeLimit(r, &in, 256<<10); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.comp.PortalSave(r.Context(), chi.URLParam(r, "token"), in.Answers, in.Submit)
	s.respond(w, r, 200, v, err)
}
