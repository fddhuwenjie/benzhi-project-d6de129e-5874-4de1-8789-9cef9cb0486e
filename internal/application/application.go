package application

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"fossil-provenance-ledger/internal/archive"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/store"
	"sort"
	"sync"
	"time"
)

type App struct {
	Store          *store.Store
	cursorKey      [32]byte
	auditPageMu    sync.Mutex
	auditPageKey   string
	auditPageCache AuditPage
	auditPageValid bool
}

type CaseListFilter struct { Status domain.CaseStatus; FieldLead, CurrentCustodian string; From, To *time.Time }
type CaseListPage struct { Cases []*domain.ProvenanceCase `json:"cases"`; StatusCounts map[domain.CaseStatus]int `json:"status_counts"`; NextCursor string `json:"next_cursor,omitempty"`; TotalCount int `json:"total_count"` }
func (a *App) ListCases(f CaseListFilter, cursor string, size int) (CaseListPage,error) {
	if size<=0 { size=20 }; if size>100 { return CaseListPage{}, fmt.Errorf("page_size exceeds limit") }
	all:=a.Store.List(); for _,c:=range all { domain.RefreshDerived(c) }; filtered:=make([]*domain.ProvenanceCase,0); counts:=map[domain.CaseStatus]int{}
	for _,c:= range all { if f.Status!=""&&c.Status!=f.Status {continue}; if f.FieldLead!=""&&c.FieldLead!=f.FieldLead {continue}; if f.CurrentCustodian!=""&&c.CurrentCustodian!=f.CurrentCustodian {continue}; if f.From!=nil&& c.DiscoveredAt.Before(*f.From){continue}; if f.To!=nil&&c.DiscoveredAt.After(*f.To){continue}; filtered=append(filtered,c); counts[c.Status]++ }
	start:=0; if cursor!="" { var x struct{Key string `json:"key"`; Sig string `json:"sig"`}; b,e:=base64.RawURLEncoding.DecodeString(cursor); if e!=nil||json.Unmarshal(b,&x)!=nil||x.Sig!=caseFilterKey(f)+":"+x.Key{return CaseListPage{},domain.ErrInvalidCursor}; for i,c:= range filtered { k:=caseListKey(c); if k==x.Key {start=i+1;break} }; if start==0{return CaseListPage{},domain.ErrInvalidCursor} }
	end:=start+size; if end>len(filtered){end=len(filtered)}; p:=CaseListPage{Cases:filtered[start:end],StatusCounts:counts,TotalCount:len(filtered)}; if end<len(filtered){ k:=caseListKey(filtered[end-1]); raw,_:=json.Marshal(struct{Key string `json:"key"`; Sig string `json:"sig"`}{k,caseFilterKey(f)+":"+k}); p.NextCursor=base64.RawURLEncoding.EncodeToString(raw) }; return p,nil
}
func caseListKey(c *domain.ProvenanceCase) string { t:=""; if c.CreatedAt!=nil {t=c.CreatedAt.UTC().Format(time.RFC3339Nano)}; return t+"|"+c.ID }
func caseFilterKey(f CaseListFilter) string { return fp(struct{Status domain.CaseStatus; FieldLead,CurrentCustodian string; From,To *time.Time}{f.Status,f.FieldLead,f.CurrentCustodian,f.From,f.To}) }

func New(s *store.Store) *App { a := &App{Store: s}; _, _ = rand.Read(a.cursorKey[:]); return a }
func fp(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func commandFP(id, typ string, expected uint64, actor string, payload any) string {
	return fp(struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Expected uint64 `json:"expected_revision"`
		Actor    string `json:"actor_id"`
		Payload  any    `json:"payload"`
	}{id, typ, expected, actor, payload})
}

type CreateInput struct {
	ID, SiteName, StratigraphicUnit, FieldLead, PermitReference string
	Latitude, Longitude                                         float64
	DiscoveredAt                                                time.Time
}

func (a *App) Create(in CreateInput, req, actor string) (*domain.ProvenanceCase, error) {
	c := &domain.ProvenanceCase{ID: in.ID, SiteName: in.SiteName, StratigraphicUnit: in.StratigraphicUnit, FieldLead: in.FieldLead, PermitReference: in.PermitReference, Latitude: in.Latitude, Longitude: in.Longitude, DiscoveredAt: in.DiscoveredAt, Status: domain.Draft, CreatedAt: ptr(time.Now().UTC())}
	if err := domain.ValidateCase(c); err != nil {
		return nil, err
	}
	b, e := a.Store.Create(c, req, commandFP(in.ID, "CASE_CREATED", 0, actor, in), actor)
	if e != nil {
		return nil, e
	}
	return decodeCase(b)
}
func ptr(t time.Time) *time.Time { return &t }
func decodeCase(b []byte) (*domain.ProvenanceCase, error) {
	var out domain.ProvenanceCase
	if e := json.Unmarshal(b, &out); e != nil {
		return nil, e
	}
	domain.RefreshDerived(&out)
	return &out, nil
}

func (a *App) mutate(id string, expected uint64, req, actor, typ string, payload any, fn func(*domain.ProvenanceCase) error) (*domain.ProvenanceCase, error) {
	fingerprint := commandFP(id, typ, expected, actor, payload)
	if req != "" {
		if b, ie, ok := a.Store.CheckIdem(req, fingerprint); ok {
			if ie != nil {
				return nil, ie
			}
			return decodeCase(b)
		}
	}
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	if c.Status == domain.Archived {
		return nil, domain.ErrArchived
	}
	if e = fn(c); e != nil {
		return nil, e
	}
	domain.RefreshDerived(c)
	b, e := a.Store.Commit(c, expected, req, fingerprint, actor, typ, payload)
	if e != nil {
		return nil, e
	}
	return decodeCase(b)
}

type DraftPatch struct {
	SiteName          *string    `json:"site_name,omitempty"`
	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	StratigraphicUnit *string    `json:"stratigraphic_unit,omitempty"`
	DiscoveredAt      *time.Time `json:"discovered_at,omitempty"`
	FieldLead         *string    `json:"field_lead,omitempty"`
	PermitReference   *string    `json:"permit_reference,omitempty"`
}

func (a *App) ReviseDraft(id string, expected uint64, req, actor string, p DraftPatch) (*domain.ProvenanceCase, error) {
	changes := map[string]domain.FieldChange{}
	payload := struct {
		Patch   DraftPatch                    `json:"patch"`
		Changes map[string]domain.FieldChange `json:"changes"`
	}{p, changes}
	return a.mutate(id, expected, req, actor, "CASE_DRAFT_REVISED", &payload, func(c *domain.ProvenanceCase) error {
		if c.Status != domain.Draft && c.Status != domain.ExtractionAuthorized && c.Status != domain.ProvenanceReview {
			return domain.ErrInvalidTransition
		}
		oldStatus:=c.Status
		if p.SiteName != nil {
			v := domain.Normalize(*p.SiteName)
			if v != c.SiteName {
				changes["site_name"] = domain.FieldChange{Before: c.SiteName, After: v}
				c.SiteName = v
			}
		}
		if p.Latitude != nil && *p.Latitude != c.Latitude {
			if e := domain.ValidateCoordinate(*p.Latitude, true); e != nil {
				return e
			}
			v := domain.NormalizeCoordinate(*p.Latitude)
			changes["latitude"] = domain.FieldChange{Before: c.Latitude, After: v}
			c.Latitude = v
		}
		if p.Longitude != nil && *p.Longitude != c.Longitude {
			if e := domain.ValidateCoordinate(*p.Longitude, false); e != nil {
				return e
			}
			v := domain.NormalizeCoordinate(*p.Longitude)
			changes["longitude"] = domain.FieldChange{Before: c.Longitude, After: v}
			c.Longitude = v
		}
		if p.StratigraphicUnit != nil {
			v := domain.Normalize(*p.StratigraphicUnit)
			if v != c.StratigraphicUnit {
				changes["stratigraphic_unit"] = domain.FieldChange{Before: c.StratigraphicUnit, After: v}
				c.StratigraphicUnit = v
			}
		}
		if p.DiscoveredAt != nil && !p.DiscoveredAt.Equal(c.DiscoveredAt) {
			changes["discovered_at"] = domain.FieldChange{Before: c.DiscoveredAt, After: p.DiscoveredAt.UTC()}
			c.DiscoveredAt = p.DiscoveredAt.UTC()
		}
		if p.FieldLead != nil {
			v := domain.Normalize(*p.FieldLead)
			if v != c.FieldLead {
				changes["field_lead"] = domain.FieldChange{Before: c.FieldLead, After: v}
				c.FieldLead = v
			}
		}
		if p.PermitReference != nil {
			v := domain.Normalize(*p.PermitReference)
			if v != c.PermitReference {
				changes["permit_reference"] = domain.FieldChange{Before: c.PermitReference, After: v}
				c.PermitReference = v
			}
		}
		if len(changes) == 0 {
			return domain.ErrNoChanges
		}
		if oldStatus != domain.Draft && (changes["latitude"].Before != nil || changes["longitude"].Before != nil || changes["stratigraphic_unit"].Before != nil || changes["permit_reference"].Before != nil) { c.Status = domain.Draft; c.CurrentReviewRound = 0 }
		if e := domain.ValidateCase(c); e != nil {
			return e
		}
		c.DuplicateCheckStatus = "CLEAR"
		c.DuplicateMatches = nil
		c.LastRevisedBy = domain.Normalize(actor)
		if c.LastRevisedBy == "" {
			return fmt.Errorf("%w: actor_id required", domain.ErrValidation)
		}
		return nil
	})
}

func (a *App) CorrectSpecimen(id string, expected uint64, req, actor string, field, orientation, batch, evidence string) (*domain.ProvenanceCase,error) {
	p:=map[string]any{"field_number":field,"orientation":orientation,"extraction_batch":batch,"evidence_digest":evidence}; if cur,e:=a.Store.Get(id);e==nil { for _,s:=range cur.Specimens { if s.FieldNumber==domain.Normalize(field) { p["before"]=map[string]any{"orientation":s.Orientation,"extraction_batch":s.ExtractionBatch,"evidence_digest":s.EvidenceDigest}; break } } }; return a.mutate(id,expected,req,actor,"SPECIMEN_CORRECTED",p,func(c *domain.ProvenanceCase) error{return domain.CorrectSpecimen(c,field,orientation,batch,evidence)})
}
func (a *App) RetractSpecimen(id string, expected uint64, req, actor, field, reason string) (*domain.ProvenanceCase,error) {
	if domain.Normalize(reason)=="" { return nil, fmt.Errorf("%w: 撤回原因不能为空",domain.ErrValidation) }; p:=map[string]any{"field_number":field,"reason":reason}; if cur,e:=a.Store.Get(id);e==nil { for _,s:=range cur.Specimens {if s.FieldNumber==domain.Normalize(field){p["before"]=s;break}} }; return a.mutate(id,expected,req,actor,"SPECIMEN_RETRACTED",p,func(c *domain.ProvenanceCase) error{return domain.RetractSpecimen(c,field)})
}

type BatchSnapshot struct { CaseID string `json:"case_id"`; Revision uint64 `json:"revision"`; ExtractionBatch string `json:"extraction_batch"`; RegisteredCount uint32 `json:"registered_count"`; PendingCount uint32 `json:"pending_count"`; Seals []string `json:"seals"`; RetiredSeals []string `json:"retired_seals"`; Digest string `json:"digest"`; CanTransfer bool `json:"can_transfer"` }
func (a *App) BatchSnapshot(id,batch string) (BatchSnapshot,error) { c,e:=a.Store.Get(id); if e!=nil{return BatchSnapshot{},e}; ss:=[]string{}; n:=uint32(0); for _,s:=range c.Specimens {if batch==""||s.ExtractionBatch==batch {if s.Status!="RETRACTED" {n++; ss=append(ss,s.SealCode)}}}; sort.Strings(ss); pending:=uint32(0); if c.Status==domain.ExtractionAuthorized {pending=n}; x:=BatchSnapshot{CaseID:id,Revision:c.Revision,ExtractionBatch:batch,RegisteredCount:n,PendingCount:pending,Seals:ss,RetiredSeals:append([]string(nil),c.RetiredSeals...),CanTransfer:c.Status==domain.Extracted||c.Status==domain.InTransit}; x.Digest=fp(x); return x,nil }
type TransferView struct { Transfers []domain.CustodyTransfer `json:"transfers"`; CurrentCustodian string `json:"current_custodian,omitempty"`; NextReceiver string `json:"next_receiver,omitempty"`; OpenDiscrepancies []string `json:"open_discrepancies,omitempty"`; TotalCount int `json:"total_count"` }
func (a *App) TransferView(id,actor string, seq uint32, openOnly bool)(TransferView,error){ c,e:=a.Store.Get(id); if e!=nil{return TransferView{},e}; out:=TransferView{}; for _,t:=range c.Transfers {if actor!=""&&t.FromActor!=actor&&t.ToActor!=actor{continue}; if seq>0&&t.Sequence!=seq{continue}; open:=false; for _,d:=range t.Discrepancies {if d.Status!="RESOLVED"{open=true; out.OpenDiscrepancies=append(out.OpenDiscrepancies,d.ID)}}; if openOnly&&!open{continue}; out.Transfers=append(out.Transfers,t)}; out.TotalCount=len(out.Transfers); out.CurrentCustodian=c.CurrentCustodian; if len(out.Transfers)>0 && (!openOnly || len(out.OpenDiscrepancies)==0) { out.NextReceiver=out.Transfers[len(out.Transfers)-1].ToActor }; return out,nil }
type IntakeReport struct { Batches []domain.IntakeBatchSummary `json:"batches"`; Passed uint32 `json:"passed"`; Failed uint32 `json:"failed"`; Pending uint32 `json:"pending"`; Completion float64 `json:"completion_percent"`; OpenOnly bool `json:"open_only"`; DifferenceCounts map[string]int `json:"difference_counts"`; FailedFieldNumbers []string `json:"failed_field_numbers,omitempty"` }
func (a *App) IntakeReport(id,batch,kind string, openOnly bool)(IntakeReport,error){ c,e:=a.Store.Get(id); if e!=nil{return IntakeReport{},e}; r:=IntakeReport{OpenOnly:openOnly,DifferenceCounts:map[string]int{}}; for _,s:=range c.Specimens {if batch!=""&&s.ExtractionBatch!=batch{continue}; switch s.IntakeResult{case domain.IntakePassed:r.Passed++;case domain.IntakeFailed:r.Failed++;r.FailedFieldNumbers=append(r.FailedFieldNumbers,s.FieldNumber);default:r.Pending++}; for _,d:=range s.IntakeDifferences {if kind==""||kind==d {r.DifferenceCounts[d]++}}}; total:=r.Passed+r.Failed+r.Pending; if total>0{r.Completion=float64(r.Passed)*100/float64(total)}; r.Batches=append(r.Batches,c.IntakeBatchSummary...); return r,nil }

func (a *App) SubmitReview(id string, expected uint64, req, actor string, r domain.Review) (*domain.ProvenanceCase, error) {
	r.SubmitterID = actor
	if r.Opinion == "" {
		r.Opinion = "提交复核"
	}
	return a.mutate(id, expected, req, actor, "PROVENANCE_SUBMITTED", &r, func(c *domain.ProvenanceCase) error {
		if e := domain.SubmitReview(c, r, time.Now().UTC()); e != nil {
			return e
		}
		r = c.ReviewRounds[len(c.ReviewRounds)-1]
		return nil
	})
}
func (a *App) DecideReview(id string, expected uint64, req, actor string, approve bool, reason string) (*domain.ProvenanceCase, error) {
	payload := map[string]any{"approve": approve, "reason": reason}
	if current, e := a.Store.Get(id); e == nil && len(current.ReviewRounds) > 0 {
		r := current.ReviewRounds[len(current.ReviewRounds)-1]
		payload["submitter_id"] = r.SubmitterID
		payload["predecessor_round"] = r.PredecessorRound
		payload["diff_fields"] = r.DiffFields
	}
	return a.mutate(id, expected, req, actor, "PROVENANCE_DECIDED", payload, func(c *domain.ProvenanceCase) error {
		return domain.DecideReview(c, actor, approve, reason, time.Now().UTC())
	})
}

func stableSeal(caseID, field string) string {
	clean := domain.Normalize(field)
	if domain.ValidSeal("SEAL-" + clean) {
		return "SEAL-" + clean
	}
	h := sha256.Sum256([]byte(caseID + "\x00" + field))
	return "SEAL-" + hex.EncodeToString(h[:8])
}
func (a *App) AddSpecimens(id string, expected uint64, req, actor string, specimens []domain.SpecimenRecord) (*domain.ProvenanceCase, error) {
	for i := range specimens {
		specimens[i].CaseID = id
		if specimens[i].ID == "" {
			specimens[i].ID = fmt.Sprintf("%s-%s", id, domain.Normalize(specimens[i].FieldNumber))
		}
		if domain.Normalize(specimens[i].SealCode) == "" {
			specimens[i].SealCode = stableSeal(id, specimens[i].FieldNumber)
		}
		if specimens[i].IntakeResult == "" {
			specimens[i].IntakeResult = domain.IntakePending
		}
	}
	payload := struct {
		Specimens []domain.SpecimenRecord `json:"specimens"`
	}{specimens}
	return a.mutate(id, expected, req, actor, "SPECIMENS_BATCH_ADDED", payload, func(c *domain.ProvenanceCase) error { return domain.AddSpecimens(c, specimens) })
}
func (a *App) AddSpecimen(id string, expected uint64, req, actor string, s domain.SpecimenRecord) (*domain.ProvenanceCase, error) {
	return a.AddSpecimens(id, expected, req, actor, []domain.SpecimenRecord{s})
}
func (a *App) CompleteExtractionBatches(id string, expected uint64, req, actor string, counts map[string]uint32) (*domain.ProvenanceCase, error) {
	return a.mutate(id, expected, req, actor, "EXTRACTION_COMPLETED", counts, func(c *domain.ProvenanceCase) error { return domain.CompleteExtraction(c, counts) })
}
func (a *App) CompleteExtraction(id string, expected uint64, req, actor string) (*domain.ProvenanceCase, error) {
	return a.CompleteExtractionBatches(id, expected, req, actor, nil)
}
func (a *App) ReplaceSeal(id string, expected uint64, req, actor, field, oldCode, newCode, reason string) (*domain.ProvenanceCase, error) {
	payload := map[string]string{"field_number": field, "old_seal_code": oldCode, "new_seal_code": newCode, "reason": reason}
	return a.mutate(id, expected, req, actor, "SPECIMEN_SEAL_REPLACED", payload, func(c *domain.ProvenanceCase) error {
		return domain.ReplaceSeal(c, field, oldCode, newCode, reason, actor, time.Now().UTC())
	})
}

func (a *App) Transfer(id string, expected uint64, req, actor string, t domain.CustodyTransfer) (*domain.ProvenanceCase, error) {
	t.CaseID = id
	if t.SnapshotDigest != "" { snap,e:=a.BatchSnapshot(id,""); if e!=nil{return nil,e}; match:=snap.Digest==t.SnapshotDigest; if !match { if cc,e2:=a.Store.Get(id);e2==nil { seen:=map[string]bool{}; for _,sp:=range cc.Specimens { if seen[sp.ExtractionBatch]{continue}; seen[sp.ExtractionBatch]=true; if x,_:=a.BatchSnapshot(id,sp.ExtractionBatch); x.Digest==t.SnapshotDigest {match=true;break} } } }; if !match{return nil,domain.ErrSnapshotConflict} }
	if t.ID == "" {
		t.ID = fmt.Sprintf("%s-transfer-%d", id, expected+1)
	}
	return a.mutate(id, expected, req, actor, "CUSTODY_TRANSFER", &t, func(c *domain.ProvenanceCase) error { return domain.ValidateTransfer(c, &t, actor, time.Now().UTC()) })
}
func (a *App) ResolveCustodyItems(id string, expected uint64, req, actor string, resolutions []domain.DiscrepancyResolution, itemIDs []string) (*domain.ProvenanceCase, []string, error) {
	byID := map[string]domain.DiscrepancyResolution{}
	for i, r := range resolutions {
		if i < len(itemIDs) {
			byID[itemIDs[i]] = r
		}
	}
	var remaining []string
	c, e := a.mutate(id, expected, req, actor, "CUSTODY_RESOLVED", struct {
		IDs         []string                       `json:"item_ids"`
		Resolutions []domain.DiscrepancyResolution `json:"resolutions"`
	}{itemIDs, resolutions}, func(c *domain.ProvenanceCase) error {
		var err error
		remaining, err = domain.ResolveCustody(c, actor, byID, time.Now().UTC())
		return err
	})
	return c, remaining, e
}
func (a *App) ResolveCustody(id string, expected uint64, req, actor, note string) (*domain.ProvenanceCase, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	if len(c.Transfers) == 0 {
		return nil, domain.ErrValidation
	}
	t := c.Transfers[len(c.Transfers)-1]
	ids := make([]string, 0, len(t.Discrepancies))
	rs := make([]domain.DiscrepancyResolution, 0, len(t.Discrepancies))
	for _, d := range t.Discrepancies {
		ids = append(ids, d.ID)
		r := domain.DiscrepancyResolution{ResolutionType: "LEGACY_VERIFIED", Note: note, VerifiedStatus: domain.SealIntact}
		if d.DeclaredCount != nil {
			x := *d.DeclaredCount
			r.VerifiedCount = &x
		}
		rs = append(rs, r)
	}
	out, _, e := a.ResolveCustodyItems(id, expected, req, actor, rs, ids)
	return out, e
}

func (a *App) IntakeItems(id string, expected uint64, req, actor string, items []domain.IntakeItem) (*domain.ProvenanceCase, error) {
	return a.mutate(id, expected, req, actor, "INTAKE_CHECKED", items, func(c *domain.ProvenanceCase) error { return domain.CheckIntake(c, actor, items, time.Now().UTC()) })
}
func (a *App) Intake(id string, expected uint64, req, actor string, in domain.Intake) (*domain.ProvenanceCase, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	bySeal := map[string]domain.SpecimenRecord{}
	for _, s := range c.Specimens {
		bySeal[s.SealCode] = s
	}
	items := make([]domain.IntakeItem, 0, len(in.SealCodes))
	for _, code := range in.SealCodes {
		if s, ok := bySeal[code]; ok {
			items = append(items, domain.IntakeItem{FieldNumber: s.FieldNumber, ReceivedSealCode: code, ReceivedCondition: "完好", EvidenceDigest: s.EvidenceDigest})
		}
	}
	return a.IntakeItems(id, expected, req, actor, items)
}
func (a *App) ResolveIntakeItems(id string, expected uint64, req, actor string, resolutions []domain.IntakeResolution) (*domain.ProvenanceCase, error) {
	return a.mutate(id, expected, req, actor, "INTAKE_RESOLVED", resolutions, func(c *domain.ProvenanceCase) error {
		return domain.ResolveIntake(c, actor, resolutions, time.Now().UTC())
	})
}
func (a *App) ResolveIntake(id string, expected uint64, req, actor, note string) (*domain.ProvenanceCase, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	var rs []domain.IntakeResolution
	for _, s := range c.Specimens {
		if s.IntakeResult != domain.IntakePassed {
			rs = append(rs, domain.IntakeResolution{FieldNumber: s.FieldNumber, Type: "LEGACY_RESOLUTION", Note: note})
		}
	}
	return a.ResolveIntakeItems(id, expected, req, actor, rs)
}

func (a *App) Archive(id string, expected uint64, req, actor string) (*domain.ProvenanceCase, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	fingerprint := commandFP(id, "CASE_ARCHIVED", expected, actor, nil)
	if req != "" {
		if b, ie, ok := a.Store.CheckIdem(req, fingerprint); ok {
			if ie != nil {
				return nil, ie
			}
			return decodeCase(b)
		}
	}
	if c.Status != domain.LabAccepted {
		return nil, domain.ErrInvalidTransition
	}
	pre := &domain.ArchivePreflight{Passed: true, FieldCount: len(c.Specimens), EventCount: len(a.Store.Events(id))}
	for _, s := range c.Specimens {
		if s.IntakeResult != domain.IntakePassed {
			pre.FailedSpecimens = append(pre.FailedSpecimens, s.FieldNumber)
		}
	}
	for _, t := range c.Transfers {
		for _, d := range t.Discrepancies {
			if d.Status != "RESOLVED" {
				pre.OpenDiscrepancies = append(pre.OpenDiscrepancies, d.ID)
			}
		}
	}
	pre.Passed = len(pre.FailedSpecimens) == 0 && len(pre.OpenDiscrepancies) == 0
	if !pre.Passed {
		c.ArchivePreflight = pre
		return nil, fmt.Errorf("archive preflight failed: %v", pre)
	}
	events := archive.StableEvents(a.Store.Events(id))
	if !archive.VerifyChain(events) {
		return nil, fmt.Errorf("摘要链校验失败")
	}
	if uint64(len(events)) != expected {
		return nil, fmt.Errorf("摘要链校验失败")
	}
	archiveTime := time.Now().UTC()
	prev := ""
	if len(events) > 0 {
		prev = events[len(events)-1].Digest
	}
	projected := domain.AuditEvent{EventID: fmt.Sprintf("%s-%d", id, expected+1), CaseID: id, Revision: expected + 1, EventType: "CASE_ARCHIVED", ActorID: actor, RequestID: req, OccurredAt: archiveTime, PayloadJSON: []byte("null"), PreviousDigest: prev}
	projected.Digest = archive.EventDigest(projected)
	allEvents := append(events, projected)
	if !archive.VerifyChain(allEvents) {
		return nil, fmt.Errorf("摘要链校验失败")
	}
	c.Status = domain.Archived
	c.Revision = expected + 1
	c.ArchivedAt = ptr(archiveTime)
	domain.RefreshDerived(c)
	b, d, e := archive.BuildManifest(c, allEvents)
	if e != nil {
		return nil, e
	}
	c.ArchiveJSON = b
	c.ArchiveDigest = d
	c.Revision = expected
	resp, e := a.Store.Commit(c, expected, req, fingerprint, actor, "CASE_ARCHIVED", nil)
	if e != nil {
		return nil, e
	}
	return decodeCase(resp)
}

func (a *App) Get(id string) (*domain.ProvenanceCase, error) {
	c, e := a.Store.Get(id)
	if e == nil {
		domain.RefreshDerived(c)
		if c.Status != domain.Archived {
			if p, pe := a.ArchivePreflight(id); pe == nil {
				c.ArchivePreflight = p
			}
		}
	}
	return c, e
}
func (a *App) Audit(id string, offset, limit int) ([]domain.AuditEvent, bool, error) {
	if _, e := a.Store.Get(id); e != nil {
		return nil, false, e
	}
	es := archive.StableEvents(a.Store.Events(id))
	if !archive.VerifyChain(es) {
		return nil, false, fmt.Errorf("摘要链校验失败")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset > len(es) {
		offset = len(es)
	}
	end := offset + limit
	if end > len(es) {
		end = len(es)
	}
	return es[offset:end], end < len(es), nil
}

type auditCursor struct {
	CaseID         string `json:"case_id"`
	NextRevision   uint64 `json:"next_revision"`
	PreviousDigest string `json:"previous_digest"`
	Signature      string `json:"signature"`
}
type AuditPage struct {
	Events     []domain.AuditEvent `json:"events"`
	NextCursor string              `json:"next_cursor,omitempty"`
	ChainValid bool                `json:"chain_valid"`
	HasMore    bool                `json:"has_more"`
	TotalCount int                 `json:"total_count,omitempty"`
	TypeCounts map[string]int      `json:"type_counts,omitempty"`
}

type AuditFilter struct {
	ActorID, EventType       string
	MinRevision, MaxRevision uint64
	From, To                 *time.Time
}

func (a *App) AuditFiltered(id string, f AuditFilter, cursor string, limit int) (AuditPage, error) {
	if _, e := a.Store.Get(id); e != nil {
		return AuditPage{}, e
	}
	all := archive.StableEvents(a.Store.Events(id))
	if !archive.VerifyChain(all) {
		return AuditPage{ChainValid: false}, fmt.Errorf("摘要链校验失败")
	}
	filtered := make([]domain.AuditEvent, 0)
	counts := map[string]int{}
	for _, e := range all {
		if f.ActorID != "" && e.ActorID != f.ActorID {
			continue
		}
		if f.EventType != "" && e.EventType != f.EventType {
			continue
		}
		if f.MinRevision > 0 && e.Revision < f.MinRevision {
			continue
		}
		if f.MaxRevision > 0 && e.Revision > f.MaxRevision {
			continue
		}
		if f.From != nil && e.OccurredAt.Before(*f.From) {
			continue
		}
		if f.To != nil && e.OccurredAt.After(*f.To) {
			continue
		}
		filtered = append(filtered, e)
		counts[e.EventType]++
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	if cursor != "" {
		x, e := a.decodeCursorFiltered(cursor, id, f)
		if e != nil {
			return AuditPage{}, e
		}
		for i, v := range filtered {
			if v.Revision >= x.NextRevision {
				start = i
				break
			}
			start = len(filtered)
		}
		if start > 0 && x.PreviousDigest != "" && filtered[start-1].Digest != x.PreviousDigest {
			return AuditPage{}, domain.ErrInvalidCursor
		}
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := AuditPage{Events: filtered[start:end], ChainValid: true, HasMore: end < len(filtered), TotalCount: len(filtered), TypeCounts: counts}
	if page.HasMore {
		last := filtered[end-1]
		page.NextCursor = a.encodeCursorFiltered(id, last.Revision+1, last.Digest, f)
	}
	return page, nil
}
func (a *App) cursorFilterKey(f AuditFilter) string { b, _ := json.Marshal(f); return fp(string(b)) }
func (a *App) encodeCursorFiltered(id string, next uint64, prev string, f AuditFilter) string {
	x := auditCursor{CaseID: id, NextRevision: next, PreviousDigest: prev, Signature: a.cursorSignature(id, next, prev) + ":" + a.cursorFilterKey(f)}
	b, _ := json.Marshal(x)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (a *App) decodeCursorFiltered(raw, id string, f AuditFilter) (auditCursor, error) {
	var x auditCursor
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || json.Unmarshal(b, &x) != nil || x.CaseID != id {
		return x, domain.ErrInvalidCursor
	}
	want := a.cursorSignature(id, x.NextRevision, x.PreviousDigest) + ":" + a.cursorFilterKey(f)
	if subtle.ConstantTimeCompare([]byte(want), []byte(x.Signature)) != 1 {
		return x, domain.ErrInvalidCursor
	}
	return x, nil
}

func (a *App) ArchivePreflight(id string) (*domain.ArchivePreflight, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, e
	}
	es := archive.StableEvents(a.Store.Events(id))
	r := &domain.ArchivePreflight{Passed: true, FieldCount: len(c.Specimens), EventCount: len(es)}
	for _, ev := range es {
		if archive.EventDigest(ev) != ev.Digest {
			r.DigestMismatches = append(r.DigestMismatches, ev.EventID)
		}
	}
	if c.Status != domain.LabAccepted {
		r.MissingFields = append(r.MissingFields, "status")
	}
	for _, s := range c.Specimens {
		if s.IntakeResult != domain.IntakePassed {
			r.FailedSpecimens = append(r.FailedSpecimens, s.FieldNumber)
		}
	}
	for _, t := range c.Transfers {
		for _, d := range t.Discrepancies {
			if d.Status != "RESOLVED" {
				r.OpenDiscrepancies = append(r.OpenDiscrepancies, d.ID)
			}
		}
	}
	r.Passed = len(r.MissingFields) == 0 && len(r.FailedSpecimens) == 0 && len(r.OpenDiscrepancies) == 0 && len(r.DigestMismatches) == 0 && archive.VerifyChain(es) && uint64(len(es)) == c.Revision
	if r.Passed {
		projected := append(es, domain.AuditEvent{CaseID: id, Revision: c.Revision + 1, EventType: "CASE_ARCHIVED", ActorID: "", PayloadJSON: []byte("null"), PreviousDigest: lastDigest(es)})
		projected[len(projected)-1].Digest = archive.EventDigest(projected[len(projected)-1])
		projectedCase := *c
		projectedCase.Status = domain.Archived
		projectedCase.Revision = c.Revision + 1
		projectedCase.ArchivedAt = ptr(time.Now().UTC())
		_, d, _ := archive.BuildManifest(&projectedCase, projected)
		r.ExpectedDigest = d
	}
	c.ArchivePreflight = r
	return r, nil
}
func lastDigest(es []domain.AuditEvent) string {
	if len(es) == 0 {
		return ""
	}
	return es[len(es)-1].Digest
}

func (a *App) cursorSignature(caseID string, next uint64, prev string) string {
	h := sha256.New()
	h.Write(a.cursorKey[:])
	fmt.Fprintf(h, "%s\x00%d\x00%s", caseID, next, prev)
	return hex.EncodeToString(h.Sum(nil))
}
func (a *App) encodeCursor(caseID string, next uint64, prev string) string {
	x := auditCursor{CaseID: caseID, NextRevision: next, PreviousDigest: prev}
	x.Signature = a.cursorSignature(caseID, next, prev)
	b, _ := json.Marshal(x)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (a *App) decodeCursor(raw, caseID string) (auditCursor, error) {
	var x auditCursor
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || json.Unmarshal(b, &x) != nil || x.CaseID != caseID {
		return x, domain.ErrInvalidCursor
	}
	want := a.cursorSignature(x.CaseID, x.NextRevision, x.PreviousDigest)
	if subtle.ConstantTimeCompare([]byte(want), []byte(x.Signature)) != 1 {
		return x, domain.ErrInvalidCursor
	}
	return x, nil
}
func (a *App) AuditWithCursor(id, cursor string, limit int) (AuditPage, error) {
	if _, e := a.Store.Get(id); e != nil {
		return AuditPage{}, e
	}
	events := archive.StableEvents(a.Store.Events(id))
	if !archive.VerifyChain(events) {
		return AuditPage{ChainValid: false}, fmt.Errorf("摘要链校验失败")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	cacheKey := fp(struct {
		CaseID, Cursor, Digest string
		Limit                  int
	}{id, cursor, lastDigest(events), limit})
	a.auditPageMu.Lock()
	if a.auditPageValid && a.auditPageKey == cacheKey {
		cached := a.auditPageCache
		a.auditPageMu.Unlock()
		return cached, nil
	}
	a.auditPageMu.Unlock()
	next := uint64(1)
	prev := ""
	if cursor != "" {
		x, e := a.decodeCursor(cursor, id)
		if e != nil {
			return AuditPage{}, e
		}
		next = x.NextRevision
		prev = x.PreviousDigest
		if next < 1 || int(next-1) > len(events) {
			return AuditPage{}, domain.ErrInvalidCursor
		}
		if next > 1 && (int(next-2) >= len(events) || events[next-2].Digest != prev) {
			return AuditPage{}, domain.ErrInvalidCursor
		}
	}
	start := sort.Search(len(events), func(i int) bool { return events[i].Revision >= next })
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	page := AuditPage{Events: events[start:end], ChainValid: true, HasMore: end < len(events)}
	if page.HasMore {
		last := events[end-1]
		page.NextCursor = a.encodeCursor(id, last.Revision+1, last.Digest)
	}
	a.auditPageMu.Lock()
	a.auditPageKey = cacheKey
	a.auditPageCache = page
	a.auditPageValid = true
	a.auditPageMu.Unlock()
	return page, nil
}
func (a *App) Manifest(id string) ([]byte, string, error) {
	c, e := a.Store.Get(id)
	if e != nil {
		return nil, "", e
	}
	if c.Status != domain.Archived {
		return nil, "", fmt.Errorf("not archived")
	}
	return append([]byte(nil), c.ArchiveJSON...), c.ArchiveDigest, nil
}
