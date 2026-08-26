package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/store"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	App *application.App
	Mux *http.ServeMux
}

func stableID(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(h[:16])
}
func New(a *application.App) *Server {
	s := &Server{App: a, Mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) routes() {
	s.Mux.HandleFunc("/healthz", s.Health)
	s.Mux.HandleFunc("/v1/cases", s.Cases)
	s.Mux.HandleFunc("/v1/cases/", s.CaseAction)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errWrite(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	ec := "validation_error"
	if strings.Contains(e.Error(), "storage_unhealthy") {
		code = http.StatusServiceUnavailable
		ec = "storage_unhealthy"
	}
	switch {
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		code = http.StatusRequestTimeout
		ec = "request_canceled"
	case errors.Is(e, store.ErrNotFound):
		code = http.StatusNotFound
		ec = "not_found"
	case errors.Is(e, store.ErrRevision):
		code = http.StatusConflict
		ec = "revision_conflict"
	case errors.Is(e, store.ErrIdempotency):
		code = http.StatusConflict
		ec = "idempotency_conflict"
	case errors.Is(e, domain.ErrArchived):
		code = http.StatusConflict
		ec = "archived"
	case errors.Is(e, domain.ErrInvalidTransition):
		code = http.StatusConflict
		ec = "invalid_transition"
	case errors.Is(e, domain.ErrNoChanges):
		ec = "no_changes"
	case errors.Is(e, domain.ErrDuplicate):
		code = http.StatusConflict
		ec = "duplicate_case"
	case errors.Is(e, domain.ErrEvidenceUnchanged):
		ec = "evidence_unchanged"
	case errors.Is(e, domain.ErrRetiredSeal):
		code = http.StatusConflict
		ec = "retired_seal"
	case errors.Is(e, domain.ErrInvalidCursor):
		code = http.StatusBadRequest
		ec = "invalid_cursor"
	case errors.Is(e, domain.ErrSnapshotConflict):
		code = http.StatusConflict
		ec = "snapshot_conflict"
	}
	write(w, code, map[string]any{"error": ec, "message": e.Error()})
}
func decode(r *http.Request, v any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("content-type must be application/json")
	}
	if r.Body == nil {
		return errors.New("empty body")
	}
	d := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return errors.New("请求体只能包含一个 JSON 值")
	}
	return nil
}
func requireCommand(c commandFields) error {
	if strings.TrimSpace(c.RequestID) == "" {
		return errors.New("request_id required")
	}
	if strings.TrimSpace(c.ActorID) == "" {
		return errors.New("actor_id required")
	}
	if c.ExpectedRevision == 0 {
		return errors.New("expected_revision required")
	}
	return nil
}
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ok, _ := s.App.Store.Healthy()
	if !ok {
		write(w, http.StatusServiceUnavailable, map[string]string{"status": "storage_unhealthy"})
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "ok", "storage": "writable"})
}

type createReq struct {
	ID                string  `json:"id"`
	SiteName          string  `json:"site_name"`
	StratigraphicUnit string  `json:"stratigraphic_unit"`
	FieldLead         string  `json:"field_lead"`
	PermitReference   string  `json:"permit_reference"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	DiscoveredAt      string  `json:"discovered_at"`
	RequestID         string  `json:"request_id"`
	ActorID           string  `json:"actor_id"`
}

func (s *Server) Cases(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listCases(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var q createReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	t, e := time.Parse(time.RFC3339Nano, q.DiscoveredAt)
	if e != nil {
		errWrite(w, errors.New("discovered_at 必须是 RFC3339 时间"))
		return
	}
	if q.ID == "" {
		q.ID = stableID("case:" + q.RequestID)
	}
	if strings.TrimSpace(q.RequestID) == "" || strings.TrimSpace(q.ActorID) == "" {
		errWrite(w, errors.New("request_id and actor_id required"))
		return
	}
	c, e := s.App.Create(application.CreateInput{ID: q.ID, SiteName: q.SiteName, StratigraphicUnit: q.StratigraphicUnit, FieldLead: q.FieldLead, PermitReference: q.PermitReference, Latitude: q.Latitude, Longitude: q.Longitude, DiscoveredAt: t}, q.RequestID, q.ActorID)
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusCreated, c)
}
func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := application.CaseListFilter{Status: domain.CaseStatus(q.Get("status")), FieldLead: q.Get("field_lead"), CurrentCustodian: q.Get("current_custodian")}
	if f.Status != "" {
		valid := map[domain.CaseStatus]bool{domain.Draft: true, domain.ProvenanceReview: true, domain.ExtractionAuthorized: true, domain.Extracted: true, domain.CustodyHold: true, domain.InTransit: true, domain.IntakeHold: true, domain.LabAccepted: true, domain.Archived: true}
		if !valid[f.Status] {
			errWrite(w, errors.New("invalid status"))
			return
		}
	}
	var e error
	var size = defaultPageSize
	if v := q.Get("page_size"); v != "" {
		size, e = strconv.Atoi(v)
		if e != nil || size <= 0 || size > 100 {
			errWrite(w, errors.New("page_size invalid"))
			return
		}
	}
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, er := time.Parse(time.RFC3339Nano, v)
		if er != nil {
			errWrite(w, er)
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, er := time.Parse(time.RFC3339Nano, v)
		if er != nil {
			errWrite(w, er)
			return
		}
		to = &t
	}
	if from != nil && to != nil && from.After(*to) {
		errWrite(w, errors.New("时间范围无效"))
		return
	}
	f.From, f.To = from, to
	p, e := s.App.ListCases(f, q.Get("cursor"), size)
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, p)
}

type commandFields struct {
	RequestID        string `json:"request_id"`
	ActorID          string `json:"actor_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}
type draftReq struct {
	commandFields
	SiteName          *string  `json:"site_name,omitempty"`
	Latitude          *float64 `json:"latitude,omitempty"`
	Longitude         *float64 `json:"longitude,omitempty"`
	StratigraphicUnit *string  `json:"stratigraphic_unit,omitempty"`
	DiscoveredAt      *string  `json:"discovered_at,omitempty"`
	FieldLead         *string  `json:"field_lead,omitempty"`
	PermitReference   *string  `json:"permit_reference,omitempty"`
}
type reviewReq struct {
	commandFields
	ProfileDescription string `json:"profile_description"`
	PhotoDigest        string `json:"photo_digest"`
	Opinion            string `json:"opinion"`
}
type decisionReq struct {
	commandFields
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}
type specimenReq struct {
	FieldNumber     string `json:"field_number"`
	Orientation     string `json:"orientation"`
	ExtractionBatch string `json:"extraction_batch"`
	EvidenceDigest  string `json:"evidence_digest"`
	SealCode        string `json:"seal_code,omitempty"`
}
type specimensReq struct {
	commandFields
	Action            string        `json:"action,omitempty"`
	Reason            string        `json:"reason,omitempty"`
	Specimens         []specimenReq `json:"specimens,omitempty"`
	FieldNumber       string        `json:"field_number,omitempty"`
	Orientation       string        `json:"orientation,omitempty"`
	ExtractionBatch   string        `json:"extraction_batch,omitempty"`
	EvidenceDigest    string        `json:"evidence_digest,omitempty"`
	SealCode          string        `json:"seal_code,omitempty"`
	ReceivedCondition string        `json:"received_condition,omitempty"`
}
type extractionReq struct {
	commandFields
	BatchCounts map[string]uint32 `json:"batch_counts,omitempty"`
}
type sealReq struct {
	commandFields
	FieldNumber string `json:"field_number"`
	OldSealCode string `json:"old_seal_code"`
	NewSealCode string `json:"new_seal_code"`
	Reason      string `json:"reason"`
}
type transferReq struct {
	commandFields
	FromActor      string            `json:"from_actor"`
	ToActor        string            `json:"to_actor"`
	TransferredAt  string            `json:"transferred_at,omitempty"`
	DeclaredCount  uint32            `json:"declared_count"`
	ReceivedCount  uint32            `json:"received_count"`
	SealStatus     domain.SealStatus `json:"seal_status"`
	AffectedSeals  []string          `json:"affected_seals,omitempty"`
	SnapshotDigest string            `json:"snapshot_digest,omitempty"`
	Snapshot       string            `json:"snapshot,omitempty"`
}
type discrepancyResolutionReq struct {
	DiscrepancyID      string            `json:"discrepancy_id"`
	ResolutionType     string            `json:"resolution_type"`
	Note               string            `json:"note"`
	VerifiedCount      *uint32           `json:"verified_count,omitempty"`
	VerifiedSealStatus domain.SealStatus `json:"verified_seal_status,omitempty"`
}
type custodyResolveReq struct {
	commandFields
	Resolutions []discrepancyResolutionReq `json:"resolutions,omitempty"`
	Note        string                     `json:"note,omitempty"`
}
type intakeItemReq struct {
	FieldNumber       string `json:"field_number"`
	ReceivedSealCode  string `json:"received_seal_code"`
	ReceivedCondition string `json:"received_condition"`
	EvidenceDigest    string `json:"evidence_digest"`
}
type intakeReq struct {
	commandFields
	Items                []intakeItemReq `json:"items,omitempty"`
	ReceivedCount        uint32          `json:"received_count,omitempty"`
	SealCodes            []string        `json:"seal_codes,omitempty"`
	EvidenceDigestIntake string          `json:"evidence_digest_intake,omitempty"`
}
type intakeResolutionReq struct {
	FieldNumber string `json:"field_number"`
	Type        string `json:"type"`
	Note        string `json:"note"`
}
type intakeResolveReq struct {
	commandFields
	Resolutions []intakeResolutionReq `json:"resolutions,omitempty"`
	Note        string                `json:"note,omitempty"`
}
type archiveReq struct{ commandFields }
type specimenCorrectionReq struct {
	commandFields
	FieldNumber     string `json:"field_number"`
	Orientation     string `json:"orientation"`
	ExtractionBatch string `json:"extraction_batch"`
	EvidenceDigest  string `json:"evidence_digest"`
}
type specimenRetractReq struct {
	commandFields
	FieldNumber string `json:"field_number"`
	Reason      string `json:"reason"`
}

func parseAction(path string) (string, string, bool) {
	p := strings.TrimPrefix(path, "/v1/cases/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}
func (s *Server) CaseAction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseAction(r.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if action == "" {
		if r.Method == http.MethodGet {
			s.getCase(w, id)
			return
		}
		if r.Method == http.MethodPatch {
			s.revise(w, r, id)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodGet {
		switch action {
		case "batches":
			s.batches(w, r, id)
			return
		case "transfers":
			s.transfers(w, r, id)
			return
		case "intake-report":
			s.intakeReport(w, r, id)
			return
		}
	}
	if r.Method == http.MethodGet && (action == "audit" || action == "manifest" || action == "archive-preflight" || action == "preflight" || action == "batches" || action == "batch-dashboard") {
		if action == "audit" {
			s.audit(w, r, id)
		} else if action == "manifest" {
			s.manifest(w, id)
		} else if action == "batches" || action == "batch-dashboard" {
			s.batches(w, r, id)
		} else {
			s.preflight(w, id)
		}
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "revise", "draft-revision", "revision":
		s.revise(w, r, id)
	case "review":
		s.review(w, r, id)
	case "review-decision":
		s.decide(w, r, id)
	case "specimens":
		s.specimens(w, r, id)
	case "extraction-complete", "extraction":
		s.extraction(w, r, id)
	case "seal-replace", "specimen-seal":
		s.replaceSeal(w, r, id)
	case "specimen-correction", "specimen-correct":
		s.correctSpecimen(w, r, id)
	case "specimen-retract", "specimen-withdraw":
		s.retractSpecimen(w, r, id)
	case "transfers":
		s.transfer(w, r, id)
	case "custody-resolve":
		s.resolveCustody(w, r, id)
	case "intake":
		s.intake(w, r, id)
	case "intake-resolve":
		s.resolveIntake(w, r, id)
	case "archive":
		s.archive(w, r, id)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
func (s *Server) getCase(w http.ResponseWriter, id string) {
	c, e := s.App.Get(id)
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, c)
}
func (s *Server) revise(w http.ResponseWriter, r *http.Request, id string) {
	var q draftReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	p := application.DraftPatch{SiteName: q.SiteName, Latitude: q.Latitude, Longitude: q.Longitude, StratigraphicUnit: q.StratigraphicUnit, FieldLead: q.FieldLead, PermitReference: q.PermitReference}
	if q.DiscoveredAt != nil {
		t, e := time.Parse(time.RFC3339Nano, *q.DiscoveredAt)
		if e != nil {
			errWrite(w, errors.New("discovered_at 必须是 RFC3339 时间"))
			return
		}
		p.DiscoveredAt = &t
	}
	c, e := s.App.ReviseDraft(id, q.ExpectedRevision, q.RequestID, q.ActorID, p)
	respondCase(w, c, e)
}
func (s *Server) review(w http.ResponseWriter, r *http.Request, id string) {
	var q reviewReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if len([]rune(strings.TrimSpace(q.ProfileDescription))) > 4096 || len([]rune(strings.TrimSpace(q.Opinion))) > 4096 {
		errWrite(w, errors.New("复核文本超出长度限制"))
		return
	}
	if len(q.PhotoDigest) != 64 || q.PhotoDigest != strings.ToLower(q.PhotoDigest) || strings.Trim(q.PhotoDigest, "0123456789abcdef") != "" {
		errWrite(w, errors.New("photo_digest 必须是六十四位小写十六进制摘要"))
		return
	}
	c, e := s.App.SubmitReview(id, q.ExpectedRevision, q.RequestID, q.ActorID, domain.Review{ProfileDescription: q.ProfileDescription, PhotoDigest: q.PhotoDigest, Opinion: q.Opinion})
	respondCase(w, c, e)
}
func (s *Server) decide(w http.ResponseWriter, r *http.Request, id string) {
	var q decisionReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	c, e := s.App.DecideReview(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.Approve, q.Reason)
	respondCase(w, c, e)
}
func (s *Server) specimens(w http.ResponseWriter, r *http.Request, id string) {
	var q specimensReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if q.Action == "correction" || q.Action == "correct" {
		c, e := s.App.CorrectSpecimen(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.FieldNumber, q.Orientation, q.ExtractionBatch, q.EvidenceDigest)
		respondCase(w, c, e)
		return
	}
	if q.Action == "retract" || q.Action == "withdraw" {
		c, e := s.App.RetractSpecimen(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.FieldNumber, q.Reason)
		respondCase(w, c, e)
		return
	}
	if len(q.Specimens) == 0 && q.FieldNumber != "" {
		q.Specimens = []specimenReq{{FieldNumber: q.FieldNumber, Orientation: q.Orientation, ExtractionBatch: q.ExtractionBatch, EvidenceDigest: q.EvidenceDigest, SealCode: q.SealCode}}
	}
	items := make([]domain.SpecimenRecord, 0, len(q.Specimens))
	for _, x := range q.Specimens {
		if len(x.EvidenceDigest) != 0 && (len(x.EvidenceDigest) != 64 || x.EvidenceDigest != strings.ToLower(x.EvidenceDigest) || strings.Trim(x.EvidenceDigest, "0123456789abcdef") != "") {
			errWrite(w, errors.New("evidence_digest 必须是六十四位小写十六进制摘要"))
			return
		}
		if !domain.ValidOrientation(x.Orientation) {
			errWrite(w, errors.New("原位姿态格式无效"))
			return
		}
		items = append(items, domain.SpecimenRecord{CaseID: id, FieldNumber: x.FieldNumber, Orientation: x.Orientation, ExtractionBatch: x.ExtractionBatch, EvidenceDigest: x.EvidenceDigest, SealCode: x.SealCode, IntakeResult: domain.IntakePending})
	}
	c, e := s.App.AddSpecimens(id, q.ExpectedRevision, q.RequestID, q.ActorID, items)
	respondCase(w, c, e)
}
func (s *Server) extraction(w http.ResponseWriter, r *http.Request, id string) {
	var q extractionReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	c, e := s.App.CompleteExtractionBatches(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.BatchCounts)
	respondCase(w, c, e)
}
func (s *Server) replaceSeal(w http.ResponseWriter, r *http.Request, id string) {
	var q sealReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	c, e := s.App.ReplaceSeal(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.FieldNumber, q.OldSealCode, q.NewSealCode, q.Reason)
	respondCase(w, c, e)
}
func (s *Server) transfer(w http.ResponseWriter, r *http.Request, id string) {
	var q transferReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	var at time.Time
	if q.TransferredAt != "" {
		var e error
		at, e = time.Parse(time.RFC3339Nano, q.TransferredAt)
		if e != nil {
			errWrite(w, errors.New("transferred_at 必须是 RFC3339 时间"))
			return
		}
	}
	if q.SnapshotDigest == "" {
		q.SnapshotDigest = q.Snapshot
	}
	c, e := s.App.Transfer(id, q.ExpectedRevision, q.RequestID, q.ActorID, domain.CustodyTransfer{CaseID: id, FromActor: q.FromActor, ToActor: q.ToActor, TransferredAt: at, DeclaredCount: q.DeclaredCount, ReceivedCount: q.ReceivedCount, SealStatus: q.SealStatus, AffectedSeals: q.AffectedSeals, SnapshotDigest: q.SnapshotDigest})
	respondCase(w, c, e)
}
func (s *Server) resolveCustody(w http.ResponseWriter, r *http.Request, id string) {
	var q custodyResolveReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if len(q.Resolutions) == 0 {
		c, e := s.App.ResolveCustody(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.Note)
		respondCase(w, c, e)
		return
	}
	ids := make([]string, 0, len(q.Resolutions))
	rs := make([]domain.DiscrepancyResolution, 0, len(q.Resolutions))
	for _, x := range q.Resolutions {
		ids = append(ids, x.DiscrepancyID)
		rs = append(rs, domain.DiscrepancyResolution{ResolutionType: x.ResolutionType, Note: x.Note, VerifiedCount: x.VerifiedCount, VerifiedStatus: x.VerifiedSealStatus})
	}
	c, _, e := s.App.ResolveCustodyItems(id, q.ExpectedRevision, q.RequestID, q.ActorID, rs, ids)
	respondCase(w, c, e)
}
func (s *Server) intake(w http.ResponseWriter, r *http.Request, id string) {
	var q intakeReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if len(q.Items) == 0 {
		c, e := s.App.Intake(id, q.ExpectedRevision, q.RequestID, q.ActorID, domain.Intake{ReceivedCount: q.ReceivedCount, SealCodes: q.SealCodes, EvidenceDigest: q.EvidenceDigestIntake})
		respondCase(w, c, e)
		return
	}
	items := make([]domain.IntakeItem, 0, len(q.Items))
	for _, x := range q.Items {
		items = append(items, domain.IntakeItem{FieldNumber: x.FieldNumber, ReceivedSealCode: x.ReceivedSealCode, ReceivedCondition: x.ReceivedCondition, EvidenceDigest: x.EvidenceDigest})
	}
	c, e := s.App.IntakeItems(id, q.ExpectedRevision, q.RequestID, q.ActorID, items)
	respondCase(w, c, e)
}
func (s *Server) resolveIntake(w http.ResponseWriter, r *http.Request, id string) {
	var q intakeResolveReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if len(q.Resolutions) == 0 {
		c, e := s.App.ResolveIntake(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.Note)
		respondCase(w, c, e)
		return
	}
	rs := make([]domain.IntakeResolution, 0, len(q.Resolutions))
	for _, x := range q.Resolutions {
		rs = append(rs, domain.IntakeResolution{FieldNumber: x.FieldNumber, Type: x.Type, Note: x.Note})
	}
	c, e := s.App.ResolveIntakeItems(id, q.ExpectedRevision, q.RequestID, q.ActorID, rs)
	respondCase(w, c, e)
}
func (s *Server) archive(w http.ResponseWriter, r *http.Request, id string) {
	var q archiveReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	if p, e := s.App.ArchivePreflight(id); e == nil && !p.Passed {
		write(w, http.StatusConflict, map[string]any{"error": "archive_preflight_failed", "report": p})
		return
	}
	c, e := s.App.ArchiveContext(r.Context(), id, q.ExpectedRevision, q.RequestID, q.ActorID)
	respondCase(w, c, e)
}
func (s *Server) preflight(w http.ResponseWriter, id string) {
	p, e := s.App.ArchivePreflight(id)
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, p)
}
func (s *Server) correctSpecimen(w http.ResponseWriter, r *http.Request, id string) {
	var q specimenCorrectionReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	c, e := s.App.CorrectSpecimen(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.FieldNumber, q.Orientation, q.ExtractionBatch, q.EvidenceDigest)
	respondCase(w, c, e)
}
func (s *Server) retractSpecimen(w http.ResponseWriter, r *http.Request, id string) {
	var q specimenRetractReq
	if e := decode(r, &q); e != nil {
		errWrite(w, e)
		return
	}
	if e := requireCommand(q.commandFields); e != nil {
		errWrite(w, e)
		return
	}
	c, e := s.App.RetractSpecimen(id, q.ExpectedRevision, q.RequestID, q.ActorID, q.FieldNumber, q.Reason)
	respondCase(w, c, e)
}
func (s *Server) transfers(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	seq := uint32(0)
	if v := q.Get("sequence"); v != "" {
		n, e := strconv.ParseUint(v, 10, 32)
		if e != nil {
			errWrite(w, e)
			return
		}
		seq = uint32(n)
	}
	open := q.Get("open_only") == "true"
	v, e := s.App.TransferView(id, q.Get("actor"), seq, open)
	if e != nil {
		errWrite(w, e)
		return
	}
	min, max := uint32(0), uint32(0)
	if x := q.Get("sequence_from"); x != "" {
		n, e := strconv.ParseUint(x, 10, 32)
		if e != nil {
			errWrite(w, e)
			return
		}
		min = uint32(n)
	}
	if x := q.Get("sequence_to"); x != "" {
		n, e := strconv.ParseUint(x, 10, 32)
		if e != nil {
			errWrite(w, e)
			return
		}
		max = uint32(n)
	}
	if min > 0 || max > 0 {
		f := v.Transfers[:0]
		for _, t := range v.Transfers {
			if min > 0 && t.Sequence < min {
				continue
			}
			if max > 0 && t.Sequence > max {
				continue
			}
			f = append(f, t)
		}
		v.Transfers = f
		v.TotalCount = len(f)
	}
	write(w, http.StatusOK, v)
}
func (s *Server) intakeReport(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	v, e := s.App.IntakeReport(id, q.Get("batch"), q.Get("difference_type"), q.Get("open_only") == "true")
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, v)
}
func (s *Server) batches(w http.ResponseWriter, r *http.Request, id string) {
	c, e := s.App.Get(id)
	if e != nil {
		errWrite(w, e)
		return
	}
	batch := r.URL.Query().Get("batch")
	if batch != "" {
		snap, e := s.App.BatchSnapshot(id, batch)
		if e != nil {
			errWrite(w, e)
			return
		}
		write(w, http.StatusOK, snap)
		return
	}
	write(w, http.StatusOK, map[string]any{"revision": c.Revision, "batches": c.BatchInventory})
}
func respondCase(w http.ResponseWriter, c *domain.ProvenanceCase, e error) {
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, c)
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request, id string) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			errWrite(w, errors.New("limit 必须是整数"))
			return
		}
		limit = v
	}
	if r.URL.Query().Get("actor_id") != "" || r.URL.Query().Get("event_type") != "" || r.URL.Query().Get("from_revision") != "" || r.URL.Query().Get("to_revision") != "" || r.URL.Query().Get("min_revision") != "" || r.URL.Query().Get("max_revision") != "" || r.URL.Query().Get("from") != "" || r.URL.Query().Get("to") != "" || r.URL.Query().Get("from_time") != "" || r.URL.Query().Get("to_time") != "" {
		f := application.AuditFilter{ActorID: r.URL.Query().Get("actor_id"), EventType: r.URL.Query().Get("event_type")}
		var e error
		minRaw := r.URL.Query().Get("from_revision")
		if minRaw == "" {
			minRaw = r.URL.Query().Get("min_revision")
		}
		maxRaw := r.URL.Query().Get("to_revision")
		if maxRaw == "" {
			maxRaw = r.URL.Query().Get("max_revision")
		}
		if v := minRaw; v != "" {
			f.MinRevision, e = strconv.ParseUint(v, 10, 64)
		}
		if e != nil {
			errWrite(w, e)
			return
		}
		if v := maxRaw; v != "" {
			f.MaxRevision, e = strconv.ParseUint(v, 10, 64)
		}
		if e != nil {
			errWrite(w, e)
			return
		}
		fromRaw := r.URL.Query().Get("from")
		if fromRaw == "" {
			fromRaw = r.URL.Query().Get("from_time")
		}
		toRaw := r.URL.Query().Get("to")
		if toRaw == "" {
			toRaw = r.URL.Query().Get("to_time")
		}
		if v := fromRaw; v != "" {
			t, er := time.Parse(time.RFC3339Nano, v)
			if er != nil {
				errWrite(w, er)
				return
			}
			f.From = &t
		}
		if v := toRaw; v != "" {
			t, er := time.Parse(time.RFC3339Nano, v)
			if er != nil {
				errWrite(w, er)
				return
			}
			f.To = &t
		}
		page, er := s.App.AuditFiltered(id, f, r.URL.Query().Get("cursor"), limit)
		if er != nil {
			errWrite(w, er)
			return
		}
		write(w, http.StatusOK, page)
		return
	}
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		page, e := s.App.AuditWithCursor(id, cursor, limit)
		if e != nil {
			errWrite(w, e)
			return
		}
		write(w, http.StatusOK, page)
		return
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, e := strconv.Atoi(raw)
		if e != nil {
			errWrite(w, errors.New("offset 必须是整数"))
			return
		}
		events, more, e := s.App.Audit(id, offset, limit)
		if e != nil {
			errWrite(w, e)
			return
		}
		write(w, http.StatusOK, map[string]any{"events": events, "has_more": more, "chain_valid": archiveChain(events)})
		return
	}
	page, e := s.App.AuditWithCursor(id, "", limit)
	if e != nil {
		errWrite(w, e)
		return
	}
	write(w, http.StatusOK, page)
}
func archiveChain(events []domain.AuditEvent) bool {
	if len(events) == 0 {
		return true
	}
	for i, e := range events {
		if i > 0 && e.PreviousDigest != events[i-1].Digest {
			return false
		}
	}
	return true
}
func (s *Server) manifest(w http.ResponseWriter, id string) {
	b, d, e := s.App.Manifest(id)
	if e != nil {
		errWrite(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Archive-Digest", d)
	w.Header().Set("ETag", fmt.Sprintf("\"sha256-%s\"", d))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
