package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"fossil-provenance-ledger/internal/archive"
	"fossil-provenance-ledger/internal/domain"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrRevision = errors.New("revision conflict")
var ErrIdempotency = errors.New("idempotency conflict")

type Idempotent struct {
	Fingerprint string
	Response    []byte
}
type Store struct {
	mu     sync.RWMutex
	cases  map[string]*domain.ProvenanceCase
	events map[string][]domain.AuditEvent
	idem   map[string]Idempotent
	path   string
	healthy bool
	loadErr error
}

func New(path string) *Store {
	s := &Store{cases: map[string]*domain.ProvenanceCase{}, events: map[string][]domain.AuditEvent{}, idem: map[string]Idempotent{}, path: path, healthy: true}
	if path != "" {
		if e := s.load(); e != nil { s.healthy = false; s.loadErr = e }
	}
	if e := s.IntegrityCheck(); e != nil { s.healthy = false; s.loadErr = e }
	return s
}
func cloneCase(c *domain.ProvenanceCase) *domain.ProvenanceCase {
	if c == nil {
		return nil
	}
	x := *c
	if c.CreatedAt != nil {
		created := *c.CreatedAt
		x.CreatedAt = &created
	}
	if c.ArchivedAt != nil {
		archived := *c.ArchivedAt
		x.ArchivedAt = &archived
	}
	x.ReviewRounds = append([]domain.Review(nil), c.ReviewRounds...)
	for i := range x.ReviewRounds {
		x.ReviewRounds[i].DiffFields = append([]string(nil), c.ReviewRounds[i].DiffFields...)
		if c.ReviewRounds[i].DecidedAt != nil {
			decided := *c.ReviewRounds[i].DecidedAt
			x.ReviewRounds[i].DecidedAt = &decided
		}
	}
	if c.Review != nil {
		review := *c.Review
		review.DiffFields = append([]string(nil), c.Review.DiffFields...)
		x.Review = &review
	}
	x.Specimens = append([]domain.SpecimenRecord(nil), c.Specimens...)
	for i := range x.Specimens {
		x.Specimens[i].SealHistory = append([]domain.SealReplacement(nil), c.Specimens[i].SealHistory...)
		x.Specimens[i].IntakeDifferences = append([]string(nil), c.Specimens[i].IntakeDifferences...)
	}
	x.RetiredSeals = append([]string(nil), c.RetiredSeals...)
	x.BatchInventory = append([]domain.BatchInventory(nil), c.BatchInventory...)
	x.Transfers = append([]domain.CustodyTransfer(nil), c.Transfers...)
	for i := range x.Transfers {
		x.Transfers[i].AffectedSeals = append([]string(nil), c.Transfers[i].AffectedSeals...)
	}
	x.CustodyHoldReasons = append([]domain.DiscrepancyItem(nil), c.CustodyHoldReasons...)
	if c.LastTransferAt != nil {
		transferred := *c.LastTransferAt
		x.LastTransferAt = &transferred
	}
	if c.Intake != nil {
		intake := *c.Intake
		intake.SealCodes = append([]string(nil), c.Intake.SealCodes...)
		intake.Items = append([]domain.IntakeItem(nil), c.Intake.Items...)
		intake.Resolutions = append([]domain.IntakeResolution(nil), c.Intake.Resolutions...)
		x.Intake = &intake
	}
	x.IntakeHistory = append([]domain.Intake(nil), c.IntakeHistory...)
	x.IntakeBatchSummary = append([]domain.IntakeBatchSummary(nil), c.IntakeBatchSummary...)
	x.ArchiveJSON = append([]byte(nil), c.ArchiveJSON...)
	x.DuplicateMatches = append([]string(nil), c.DuplicateMatches...)
	if c.ArchivePreflight != nil {
		preflight := *c.ArchivePreflight
		preflight.MissingFields = append([]string(nil), c.ArchivePreflight.MissingFields...)
		preflight.OpenDiscrepancies = append([]string(nil), c.ArchivePreflight.OpenDiscrepancies...)
		preflight.FailedSpecimens = append([]string(nil), c.ArchivePreflight.FailedSpecimens...)
		preflight.DigestMismatches = append([]string(nil), c.ArchivePreflight.DigestMismatches...)
		x.ArchivePreflight = &preflight
	}
	return &x
}
func (s *Store) load() error {
	b, e := os.ReadFile(s.path)
	if e != nil {
		if os.IsNotExist(e) { return nil }
		return e
	}
	var d struct {
		Cases  map[string]*domain.ProvenanceCase
		Events map[string][]domain.AuditEvent
		Idem   map[string]Idempotent
	}
	if e := json.Unmarshal(b, &d); e != nil { return e }
	if d.Cases != nil {
		s.cases = d.Cases
	}
	if d.Events != nil {
		s.events = d.Events
	}
	if d.Idem != nil {
		s.idem = d.Idem
	}
	return nil
}
func (s *Store) Healthy() (bool, error) { s.mu.RLock(); defer s.mu.RUnlock(); return s.healthy, s.loadErr }
func (s *Store) IntegrityCheck() error {
	s.mu.RLock(); defer s.mu.RUnlock()
	for id, c := range s.cases {
		es := archive.StableEvents(s.events[id])
		if c.ID != id || c.Revision != uint64(len(es)) || !archive.VerifyChain(es) { return fmt.Errorf("case %s integrity failure", id) }
	}
	for req, x := range s.idem { if req=="" || x.Fingerprint=="" { return fmt.Errorf("idempotency index invalid") }; var c domain.ProvenanceCase; if json.Unmarshal(x.Response,&c)!=nil { return fmt.Errorf("idempotency response invalid") } }
	return nil
}
func (s *Store) List() []*domain.ProvenanceCase {
	s.mu.RLock(); defer s.mu.RUnlock()
	out := make([]*domain.ProvenanceCase,0,len(s.cases)); for _, c := range s.cases { out = append(out, cloneCase(c)) }
	sort.Slice(out, func(i,j int) bool { if out[i].CreatedAt == nil || out[j].CreatedAt == nil { return out[i].ID < out[j].ID }; if out[i].CreatedAt.Equal(*out[j].CreatedAt) { return out[i].ID < out[j].ID }; return out[i].CreatedAt.Before(*out[j].CreatedAt) }); return out
}
func (s *Store) persist() {
	if s.path == "" {
		return
	}
	d := struct {
		Cases  map[string]*domain.ProvenanceCase
		Events map[string][]domain.AuditEvent
		Idem   map[string]Idempotent
	}{s.cases, s.events, s.idem}
	b, _ := json.Marshal(d)
	_ = os.WriteFile(s.path, b, 0600)
}
func (s *Store) Get(id string) (*domain.ProvenanceCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cases[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneCase(c), nil
}
func (s *Store) Events(id string) []domain.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.AuditEvent(nil), s.events[id]...)
	for i := range out {
		out[i].PayloadJSON = append([]byte(nil), out[i].PayloadJSON...)
	}
	return out
}
func (s *Store) CheckIdem(req, fp string) ([]byte, error, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.idem[req]
	if !ok {
		return nil, nil, false
	}
	if x.Fingerprint != fp {
		return nil, ErrIdempotency, true
	}
	return append([]byte(nil), x.Response...), nil, true
}
func (s *Store) Commit(c *domain.ProvenanceCase, expected uint64, req, fp string, actor, typ string, payload any) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.healthy { return nil, errors.New("storage_unhealthy") }
	if req != "" {
		if x, ok := s.idem[req]; ok {
			if x.Fingerprint != fp {
				return nil, ErrIdempotency
			}
			return append([]byte(nil), x.Response...), nil
		}
	}
	old, exists := s.cases[c.ID]
	if exists && old.Revision != expected {
		return nil, ErrRevision
	}
	if !exists && expected != 0 {
		return nil, ErrRevision
	}
	duplicateCheck := map[string]any(nil)
	if typ == "CASE_DRAFT_REVISED" {
		duplicateCheck = map[string]any{"permit_reference": c.PermitReference, "site_name": c.SiteName, "radius_meters": 10, "days": 7, "matches": []string{}}
		for id, existing := range s.cases {
			if id == c.ID || existing.Status == domain.Archived || existing.PermitReference != c.PermitReference || existing.SiteName != c.SiteName {
				continue
			}
			if haversineMeters(existing.Latitude, existing.Longitude, c.Latitude, c.Longitude) <= 10 && math.Abs(existing.DiscoveredAt.Sub(c.DiscoveredAt).Hours()) <= 24*7 {
				return nil, fmt.Errorf("%w: %s", domain.ErrDuplicate, id)
			}
		}
	}
	prev := ""
	if es := s.events[c.ID]; len(es) > 0 {
		prev = es[len(es)-1].Digest
	}
	c.Revision = expected + 1
	now := time.Now().UTC()
	if typ == "CASE_ARCHIVED" && c.ArchivedAt != nil {
		now = c.ArchivedAt.UTC()
	}
	if duplicateCheck != nil {
		var obj map[string]any
		if raw, e := json.Marshal(payload); e == nil && json.Unmarshal(raw, &obj) == nil {
			obj["duplicate_check"] = duplicateCheck
			payload = obj
		}
	}
	payloadJSON, _ := json.Marshal(payload)
	e := domain.AuditEvent{EventID: fmt.Sprintf("%s-%d", c.ID, c.Revision), CaseID: c.ID, Revision: c.Revision, EventType: typ, ActorID: actor, RequestID: req, OccurredAt: now, PayloadJSON: payloadJSON, PreviousDigest: prev}
	e.Digest = archive.EventDigest(e)
	s.cases[c.ID] = cloneCase(c)
	s.events[c.ID] = append(s.events[c.ID], e)
	resp, _ := json.Marshal(c)
	if req != "" {
		s.idem[req] = Idempotent{Fingerprint: fp, Response: resp}
	}
	s.persist()
	return resp, nil
}
func (s *Store) Create(c *domain.ProvenanceCase, req, fp, actor string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req != "" {
		if x, ok := s.idem[req]; ok {
			if x.Fingerprint != fp {
				return nil, ErrIdempotency
			}
			return append([]byte(nil), x.Response...), nil
		}
	}
	if _, exists := s.cases[c.ID]; exists {
		return nil, ErrRevision
	}
	matches := []string{}
	for id, existing := range s.cases {
		if existing.Status == domain.Archived || existing.PermitReference != c.PermitReference || existing.SiteName != c.SiteName {
			continue
		}
		d := haversineMeters(existing.Latitude, existing.Longitude, c.Latitude, c.Longitude)
		if d <= 10 && math.Abs(existing.DiscoveredAt.Sub(c.DiscoveredAt).Hours()) <= 24*7 {
			matches = append(matches, id)
		}
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return nil, fmt.Errorf("%w: %s", domain.ErrDuplicate, strings.Join(matches, ","))
	}
	c.DuplicateCheckStatus = "CLEAR"
	c.DuplicateMatches = nil
	prev := ""
	payloadJSON, _ := json.Marshal(map[string]any{"duplicate_check": map[string]any{"permit_reference": c.PermitReference, "site_name": c.SiteName, "radius_meters": 10, "days": 7, "matches": matches}})
	e := domain.AuditEvent{EventID: fmt.Sprintf("%s-1", c.ID), CaseID: c.ID, Revision: 1, EventType: "CASE_CREATED", ActorID: actor, RequestID: req, OccurredAt: time.Now().UTC(), PayloadJSON: payloadJSON, PreviousDigest: prev}
	e.Digest = archive.EventDigest(e)
	c.Revision = 1
	s.cases[c.ID] = cloneCase(c)
	s.events[c.ID] = []domain.AuditEvent{e}
	resp, _ := json.Marshal(c)
	if req != "" {
		s.idem[req] = Idempotent{Fingerprint: fp, Response: resp}
	}
	s.persist()
	return resp, nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000
	p := math.Pi / 180
	dlat := (lat2 - lat1) * p
	dlon := (lon2 - lon1) * p
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*p)*math.Cos(lat2*p)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
