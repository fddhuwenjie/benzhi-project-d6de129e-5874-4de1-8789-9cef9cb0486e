package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CaseStatus string

const (
	Draft                CaseStatus = "DRAFT"
	ProvenanceReview     CaseStatus = "PROVENANCE_REVIEW"
	ExtractionAuthorized CaseStatus = "EXTRACTION_AUTHORIZED"
	Extracted            CaseStatus = "EXTRACTED"
	CustodyHold          CaseStatus = "CUSTODY_HOLD"
	InTransit            CaseStatus = "IN_TRANSIT"
	IntakeHold           CaseStatus = "INTAKE_HOLD"
	LabAccepted          CaseStatus = "LAB_ACCEPTED"
	Archived             CaseStatus = "ARCHIVED"
)

type IntakeResult string

const (
	IntakePending IntakeResult = "PENDING"
	IntakePassed  IntakeResult = "PASSED"
	IntakeFailed  IntakeResult = "FAILED"
)

type SealStatus string

const (
	SealIntact  SealStatus = "INTACT"
	SealBroken  SealStatus = "BROKEN"
	SealMissing SealStatus = "MISSING"
)

type ProvenanceCase struct {
	ID                   string               `json:"id"`
	SiteName             string               `json:"site_name"`
	StratigraphicUnit    string               `json:"stratigraphic_unit"`
	FieldLead            string               `json:"field_lead"`
	PermitReference      string               `json:"permit_reference"`
	Latitude             float64              `json:"latitude"`
	Longitude            float64              `json:"longitude"`
	DiscoveredAt         time.Time            `json:"discovered_at"`
	Status               CaseStatus           `json:"status"`
	Revision             uint64               `json:"revision"`
	CreatedAt            *time.Time           `json:"created_at"`
	ArchivedAt           *time.Time           `json:"archived_at,omitempty"`
	LastRevisedBy        string               `json:"last_revised_by,omitempty"`
	Review               *Review              `json:"review,omitempty"`
	ReviewRounds         []Review             `json:"review_rounds"`
	CurrentReviewRound   uint32               `json:"current_review_round,omitempty"`
	Specimens            []SpecimenRecord     `json:"specimens"`
	RetiredSeals         []string             `json:"retired_seals"`
	BatchInventory       []BatchInventory     `json:"batch_inventory"`
	Transfers            []CustodyTransfer    `json:"transfers"`
	CurrentCustodian     string               `json:"current_custodian,omitempty"`
	LastTransferAt       *time.Time           `json:"last_transfer_at,omitempty"`
	NextFromActor        string               `json:"next_from_actor,omitempty"`
	CustodyHoldReasons   []DiscrepancyItem    `json:"custody_hold_reasons"`
	Intake               *Intake              `json:"intake,omitempty"`
	IntakeHistory        []Intake             `json:"intake_history"`
	IntakeBatchSummary   []IntakeBatchSummary `json:"intake_batch_summary,omitempty"`
	ArchiveDigest        string               `json:"archive_digest,omitempty"`
	ArchiveJSON          []byte               `json:"archive_json,omitempty"`
	DuplicateCheckStatus string               `json:"duplicate_check_status,omitempty"`
	DuplicateMatches     []string             `json:"duplicate_matches,omitempty"`
	ArchivePreflight     *ArchivePreflight    `json:"archive_preflight,omitempty"`
}

type Review struct {
	Round              uint32     `json:"round"`
	ProfileDescription string     `json:"profile_description"`
	PhotoDigest        string     `json:"photo_digest"`
	Opinion            string     `json:"opinion"`
	SubmitterID        string     `json:"submitter_id"`
	SubmittedAt        time.Time  `json:"submitted_at"`
	ReviewerID         string     `json:"reviewer_id,omitempty"`
	Decision           string     `json:"decision,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	DecidedAt          *time.Time `json:"decided_at,omitempty"`
	PredecessorRound   uint32     `json:"predecessor_round,omitempty"`
	DiffFields         []string   `json:"diff_fields,omitempty"`
}

type ArchivePreflight struct {
	Passed            bool     `json:"passed"`
	MissingFields     []string `json:"missing_fields,omitempty"`
	OpenDiscrepancies []string `json:"open_discrepancies,omitempty"`
	FailedSpecimens   []string `json:"failed_specimens,omitempty"`
	DigestMismatches  []string `json:"digest_mismatches,omitempty"`
	FieldCount        int      `json:"field_count"`
	EventCount        int      `json:"event_count"`
	ExpectedDigest    string   `json:"expected_digest,omitempty"`
}

type SealReplacement struct {
	OldSealCode string    `json:"old_seal_code"`
	NewSealCode string    `json:"new_seal_code"`
	Reason      string    `json:"reason"`
	ActorID     string    `json:"actor_id"`
	ReplacedAt  time.Time `json:"replaced_at"`
}
type SpecimenRecord struct {
	ID                string            `json:"id"`
	CaseID            string            `json:"case_id"`
	FieldNumber       string            `json:"field_number"`
	Orientation       string            `json:"orientation"`
	ExtractionBatch   string            `json:"extraction_batch"`
	EvidenceDigest    string            `json:"evidence_digest"`
	SealCode          string            `json:"seal_code"`
	SealHistory       []SealReplacement `json:"seal_history"`
	ReceivedCondition string            `json:"received_condition,omitempty"`
	IntakeResult      IntakeResult      `json:"intake_result"`
	IntakeDifferences []string          `json:"intake_differences"`
	Status            string            `json:"status,omitempty"`
}
type BatchInventory struct {
	ExtractionBatch string `json:"extraction_batch"`
	RegisteredCount uint32 `json:"registered_count"`
	DeclaredCount   uint32 `json:"declared_count"`
	PendingCount    uint32 `json:"pending_count"`
	CanComplete     bool   `json:"can_complete"`
}

type DiscrepancyKind string

const (
	QuantityShort  DiscrepancyKind = "QUANTITY_SHORT"
	QuantityExcess DiscrepancyKind = "QUANTITY_EXCESS"
	SealCountMismatch DiscrepancyKind = "SEAL_COUNT_MISMATCH"
	SealDamaged    DiscrepancyKind = "SEAL_DAMAGED"
	SealAbsent     DiscrepancyKind = "SEAL_MISSING"
)

type DiscrepancyResolution struct {
	ResolutionType string     `json:"resolution_type"`
	Note           string     `json:"note"`
	VerifiedCount  *uint32    `json:"verified_count,omitempty"`
	VerifiedStatus SealStatus `json:"verified_seal_status,omitempty"`
	ActorID        string     `json:"actor_id"`
	ResolvedAt     time.Time  `json:"resolved_at"`
}
type DiscrepancyItem struct {
	ID             string                 `json:"id"`
	Kind           DiscrepancyKind        `json:"kind"`
	DeclaredCount  *uint32                `json:"declared_count,omitempty"`
	ReceivedCount  *uint32                `json:"received_count,omitempty"`
	AffectedSeals  []string               `json:"affected_seals,omitempty"`
	ObservedStatus SealStatus             `json:"observed_seal_status,omitempty"`
	Status         string                 `json:"status"`
	Resolution     *DiscrepancyResolution `json:"resolution,omitempty"`
}
type CustodyTransfer struct {
	ID              string            `json:"id"`
	CaseID          string            `json:"case_id"`
	Sequence        uint32            `json:"sequence"`
	FromActor       string            `json:"from_actor"`
	ToActor         string            `json:"to_actor"`
	TransferredAt   time.Time         `json:"transferred_at"`
	DeclaredCount   uint32            `json:"declared_count"`
	ReceivedCount   uint32            `json:"received_count"`
	SealStatus      SealStatus        `json:"seal_status"`
	AffectedSeals   []string          `json:"affected_seals,omitempty"`
	Discrepancies   []DiscrepancyItem `json:"discrepancies"`
	DiscrepancyNote string            `json:"discrepancy_note,omitempty"`
	ResolutionNote  string            `json:"resolution_note,omitempty"`
	SnapshotDigest  string            `json:"snapshot_digest,omitempty"`
}

type IntakeItem struct {
	FieldNumber       string       `json:"field_number"`
	ReceivedSealCode  string       `json:"received_seal_code"`
	ReceivedCondition string       `json:"received_condition"`
	EvidenceDigest    string       `json:"evidence_digest"`
	Result            IntakeResult `json:"result"`
	Differences       []string     `json:"differences"`
}
type IntakeResolution struct {
	FieldNumber string    `json:"field_number"`
	Type        string    `json:"type"`
	Note        string    `json:"note"`
	ActorID     string    `json:"actor_id"`
	ResolvedAt  time.Time `json:"resolved_at"`
}
type Intake struct {
	ReceivedCount  uint32             `json:"received_count"`
	SealCodes      []string           `json:"seal_codes,omitempty"`
	EvidenceDigest string             `json:"evidence_digest,omitempty"`
	Result         IntakeResult       `json:"result"`
	Note           string             `json:"note,omitempty"`
	Actor          string             `json:"actor"`
	CheckedAt      time.Time          `json:"checked_at"`
	Items          []IntakeItem       `json:"items"`
	Resolutions    []IntakeResolution `json:"resolutions"`
}
type IntakeBatchSummary struct {
	ExtractionBatch string `json:"extraction_batch"`
	PassedCount     uint32 `json:"passed_count"`
	FailedCount     uint32 `json:"failed_count"`
	PendingCount    uint32 `json:"pending_count"`
	LastRound       int    `json:"last_round"`
}

type FieldChange struct {
	Before any `json:"before"`
	After  any `json:"after"`
}
type AuditEvent struct {
	EventID        string          `json:"event_id"`
	CaseID         string          `json:"case_id"`
	Revision       uint64          `json:"revision"`
	EventType      string          `json:"event_type"`
	ActorID        string          `json:"actor_id"`
	RequestID      string          `json:"request_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	PayloadJSON    json.RawMessage `json:"payload_json"`
	PreviousDigest string          `json:"previous_digest"`
	Digest         string          `json:"digest"`
}

var ErrInvalidTransition = errors.New("invalid status transition")
var ErrValidation = errors.New("validation failed")
var ErrArchived = errors.New("case archived")
var ErrNoChanges = errors.New("no actual field changes")
var ErrDuplicate = errors.New("duplicate case")
var ErrInvalidCursor = errors.New("invalid audit cursor")
var ErrEvidenceUnchanged = errors.New("evidence unchanged")
var ErrRetiredSeal = errors.New("retired seal")
var ErrSnapshotConflict = errors.New("snapshot conflict")

func validation(message string) error { return fmt.Errorf("%w: %s", ErrValidation, message) }

func ValidateTransition(from, to CaseStatus) error {
	allowed := map[CaseStatus]map[CaseStatus]bool{Draft: {ProvenanceReview: true}, ProvenanceReview: {ExtractionAuthorized: true, Draft: true}, ExtractionAuthorized: {Extracted: true}, Extracted: {InTransit: true, CustodyHold: true}, CustodyHold: {InTransit: true}, InTransit: {IntakeHold: true, LabAccepted: true}, IntakeHold: {IntakeHold: true, LabAccepted: true}, LabAccepted: {Archived: true}}
	if !allowed[from][to] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return nil
}
func Normalize(s string) string { return strings.TrimSpace(s) }
func ValidateCaseAt(c *ProvenanceCase, now time.Time) error {
	rawLat, rawLon := c.Latitude, c.Longitude
	c.ID = Normalize(c.ID)
	c.SiteName = Normalize(c.SiteName)
	c.StratigraphicUnit = Normalize(c.StratigraphicUnit)
	c.FieldLead = Normalize(c.FieldLead)
	c.PermitReference = Normalize(c.PermitReference)
	c.Latitude = NormalizeCoordinate(c.Latitude)
	c.Longitude = NormalizeCoordinate(c.Longitude)
	if c.ID == "" || c.SiteName == "" || c.StratigraphicUnit == "" || c.FieldLead == "" || c.PermitReference == "" {
		return validation("案件必填字段不得为空")
	}
	if e := ValidatePermitReference(c.PermitReference); e != nil {
		return e
	}
	if math.IsNaN(rawLat) || math.IsInf(rawLat, 0) || math.IsNaN(rawLon) || math.IsInf(rawLon, 0) {
		return validation("坐标必须是有限数值")
	}
	if math.Abs(rawLat-NormalizeCoordinate(rawLat)) > 1e-9 || math.Abs(rawLon-NormalizeCoordinate(rawLon)) > 1e-9 {
		return validation("坐标最多保留六位小数")
	}
	if c.Latitude < -90 || c.Latitude > 90 || c.Longitude < -180 || c.Longitude > 180 {
		return validation("坐标超出范围")
	}
	if c.DiscoveredAt.IsZero() || c.DiscoveredAt.After(now) {
		return validation("发现时间不得为空或晚于当前时间")
	}
	return nil
}

func NormalizeCoordinate(v float64) float64 { return math.Round(v*1e6) / 1e6 }
func ValidateCoordinate(v float64, latitude bool) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v-NormalizeCoordinate(v)) > 1e-9 {
		return validation("坐标必须是有限数值且最多保留六位小数")
	}
	if latitude && (v < -90 || v > 90) || !latitude && (v < -180 || v > 180) {
		return validation("坐标超出范围")
	}
	return nil
}
func ValidatePermitReference(s string) error {
	s = Normalize(s)
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return validation("许可引用无效")
	}
	return nil
}
func ValidateCase(c *ProvenanceCase) error { return ValidateCaseAt(c, time.Now().UTC()) }

func SubmitReview(c *ProvenanceCase, r Review, now time.Time) error {
	if c.Status != Draft {
		return ErrInvalidTransition
	}
	r.ProfileDescription = Normalize(r.ProfileDescription)
	r.PhotoDigest = Normalize(r.PhotoDigest)
	r.Opinion = Normalize(r.Opinion)
	r.SubmitterID = Normalize(r.SubmitterID)
	if r.ProfileDescription == "" || r.PhotoDigest == "" || r.Opinion == "" || r.SubmitterID == "" {
		return validation("复核证据、提交意见和提交人不得为空")
	}
	if len(c.ReviewRounds) > 0 {
		prev := c.ReviewRounds[len(c.ReviewRounds)-1]
		if prev.Decision == "REJECTED" {
			if r.Opinion == prev.Opinion {
				return validation("退回复核意见必须说明新增内容")
			}
			if !strings.Contains(r.Opinion, "补") && !strings.Contains(r.Opinion, "新增") && !strings.Contains(r.Opinion, "补充") {
				return validation("退回复核必须在意见中说明补充内容")
			}
			if r.ProfileDescription == prev.ProfileDescription && r.PhotoDigest == prev.PhotoDigest {
				return ErrEvidenceUnchanged
			}
			r.PredecessorRound = prev.Round
			if r.ProfileDescription != prev.ProfileDescription {
				r.DiffFields = append(r.DiffFields, "profile_description")
			}
			if r.PhotoDigest != prev.PhotoDigest {
				r.DiffFields = append(r.DiffFields, "photo_digest")
			}
		}
	}
	r.Round = uint32(len(c.ReviewRounds) + 1)
	r.SubmittedAt = now.UTC()
	r.Decision = ""
	r.Reason = ""
	r.ReviewerID = ""
	r.DecidedAt = nil
	c.ReviewRounds = append(c.ReviewRounds, r)
	c.Review = &c.ReviewRounds[len(c.ReviewRounds)-1]
	c.CurrentReviewRound = r.Round
	c.Status = ProvenanceReview
	return nil
}
func DecideReview(c *ProvenanceCase, actor string, approve bool, reason string, now time.Time) error {
	if c.Status != ProvenanceReview || c.CurrentReviewRound == 0 || len(c.ReviewRounds) == 0 {
		return ErrInvalidTransition
	}
	r := &c.ReviewRounds[len(c.ReviewRounds)-1]
	if r.Round != c.CurrentReviewRound || r.Decision != "" {
		return validation("当前复核轮次已决定")
	}
	actor = Normalize(actor)
	reason = Normalize(reason)
	if actor == "" || actor == c.FieldLead || actor == r.SubmitterID {
		return validation("复核人必须独立于现场负责人和本轮提交人")
	}
	if reason == "" {
		if approve {
			return validation("批准必须给出复核意见")
		}
		return validation("退回必须给出明确原因")
	}
	r.ReviewerID = actor
	r.Reason = reason
	r.DecidedAt = timePtr(now.UTC())
	if approve {
		r.Decision = "APPROVED"
		c.Status = ExtractionAuthorized
	} else {
		r.Decision = "REJECTED"
		c.Status = Draft
	}
	c.Review = r
	c.CurrentReviewRound = 0
	return nil
}

var sealPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidSeal(code string) bool { return sealPattern.MatchString(code) }
func AddSpecimens(c *ProvenanceCase, specimens []SpecimenRecord) error {
	if c.Status != ExtractionAuthorized {
		return ErrInvalidTransition
	}
	if len(specimens) == 0 {
		return validation("标本数组至少包含一项")
	}
	fields := map[string]int{}
	seals := map[string]int{}
	evidence := map[string]string{}
	for i, x := range c.Specimens {
		fields[x.FieldNumber] = -(i + 1)
		seals[x.SealCode] = -(i + 1)
		evidence[x.EvidenceDigest] = x.FieldNumber
	}
	retired := map[string]bool{}
	for _, code := range c.RetiredSeals {
		retired[code] = true
	}
	batch := ""
	for i := range specimens {
		s := &specimens[i]
		s.FieldNumber = Normalize(s.FieldNumber)
		s.Orientation = Normalize(s.Orientation)
		s.ExtractionBatch = Normalize(s.ExtractionBatch)
		s.EvidenceDigest = Normalize(s.EvidenceDigest)
		s.SealCode = Normalize(s.SealCode)
		if s.FieldNumber == "" || s.Orientation == "" || s.ExtractionBatch == "" || s.EvidenceDigest == "" {
			return validation(fmt.Sprintf("标本位置 %d 的必填字段不得为空", i))
		}
		if !ValidOrientation(s.Orientation) {
			return validation(fmt.Sprintf("标本 %s 的原位姿态无效", s.FieldNumber))
		}
		if batch == "" {
			batch = s.ExtractionBatch
		}
		if batch != s.ExtractionBatch {
			return validation("同一批量请求必须使用一致的采掘批次")
		}
		if prior, ok := evidence[s.EvidenceDigest]; ok && prior != s.FieldNumber {
			return validation(fmt.Sprintf("证据摘要 %s 不得绑定多个野外编号", s.EvidenceDigest))
		}
		evidence[s.EvidenceDigest] = s.FieldNumber
		if at, ok := fields[s.FieldNumber]; ok {
			return validation(fmt.Sprintf("重复野外编号 %s，位置 %d 与 %d", s.FieldNumber, at, i))
		}
		fields[s.FieldNumber] = i
		if !ValidSeal(s.SealCode) {
			return validation(fmt.Sprintf("标本 %s 的封签格式无效", s.FieldNumber))
		}
		if at, ok := seals[s.SealCode]; ok || retired[s.SealCode] {
			return validation(fmt.Sprintf("封签冲突 %s，位置 %d 与 %d", s.SealCode, at, i))
		}
		seals[s.SealCode] = i
		if s.IntakeResult == "" {
			s.IntakeResult = IntakePending
		}
	}
	c.Specimens = append(c.Specimens, specimens...)
	RefreshDerived(c)
	return nil
}

var orientationCodes = map[string]bool{"N": true, "S": true, "E": true, "W": true, "NE": true, "NW": true, "SE": true, "SW": true, "UP": true, "DOWN": true}

func ValidOrientation(v string) bool {
	v = strings.ToUpper(Normalize(v))
	if orientationCodes[v] {
		return true
	}
	for _, unit := range []string{"DEG", "°"} {
		if strings.HasSuffix(v, unit) {
			x, e := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, unit)), 64)
			return e == nil && x >= -360 && x <= 360
		}
	}
	return false
}
func AddSpecimen(c *ProvenanceCase, s SpecimenRecord) error {
	return AddSpecimens(c, []SpecimenRecord{s})
}
func CompleteExtraction(c *ProvenanceCase, declared map[string]uint32) error {
	if c.Status != ExtractionAuthorized || len(c.Specimens) == 0 {
		return ErrInvalidTransition
	}
	actual := map[string]uint32{}
	for _, s := range c.Specimens {
		actual[s.ExtractionBatch]++
	}
	if len(declared) == 0 {
		declared = actual
	}
	if len(declared) != len(actual) {
		return validation("采掘批次数量不一致")
	}
	var got, want uint32
	for batch, count := range actual {
		got += count
		d, ok := declared[batch]
		if !ok || d != count {
			return validation(fmt.Sprintf("采掘批次 %s 申报数量不一致", batch))
		}
		want += d
	}
	if got != want || got != uint32(len(c.Specimens)) {
		return validation("申报总数与标本名册不一致")
	}
	c.Status = Extracted
	RefreshDerived(c)
	return nil
}
func ReplaceSeal(c *ProvenanceCase, fieldNumber, oldCode, newCode, reason, actor string, now time.Time) error {
	if c.Status != ExtractionAuthorized || len(c.Transfers) > 0 {
		return ErrInvalidTransition
	}
	fieldNumber = Normalize(fieldNumber)
	oldCode = Normalize(oldCode)
	newCode = Normalize(newCode)
	reason = Normalize(reason)
	actor = Normalize(actor)
	if fieldNumber == "" || oldCode == "" || newCode == "" || reason == "" || actor == "" || !ValidSeal(newCode) {
		return validation("封签更换字段或新封签格式无效")
	}
	idx := -1
	for i, s := range c.Specimens {
		if s.FieldNumber == fieldNumber {
			idx = i
		}
		if s.SealCode == newCode && s.FieldNumber != fieldNumber {
			return validation("新封签已由本案其他标本使用")
		}
	}
	for _, x := range c.RetiredSeals {
		if x == newCode {
			return validation("新封签已经退役")
		}
	}
	if idx < 0 {
		return validation("标本不存在")
	}
	s := &c.Specimens[idx]
	if s.SealCode != oldCode {
		return validation("原封签不是标本当前有效封签")
	}
	if oldCode == newCode {
		return ErrNoChanges
	}
	s.SealHistory = append(s.SealHistory, SealReplacement{OldSealCode: oldCode, NewSealCode: newCode, Reason: reason, ActorID: actor, ReplacedAt: now.UTC()})
	s.SealCode = newCode
	c.RetiredSeals = append(c.RetiredSeals, oldCode)
	sort.Strings(c.RetiredSeals)
	return nil
}

func CorrectSpecimen(c *ProvenanceCase, field, orientation, batch, evidence string) error {
	if c.Status != ExtractionAuthorized || len(c.Transfers) > 0 { return ErrInvalidTransition }
	field, orientation, batch, evidence = Normalize(field), Normalize(orientation), Normalize(batch), Normalize(evidence)
	for i := range c.Specimens { if c.Specimens[i].FieldNumber == field {
		if !ValidOrientation(orientation) || batch == "" || evidence == "" { return validation("标本更正字段无效") }
		if c.Specimens[i].Orientation == orientation && c.Specimens[i].ExtractionBatch == batch && c.Specimens[i].EvidenceDigest == evidence { return ErrNoChanges }
		for j,s := range c.Specimens { if j != i && s.EvidenceDigest == evidence { return validation("证据摘要冲突") } }
		c.Specimens[i].Orientation, c.Specimens[i].ExtractionBatch, c.Specimens[i].EvidenceDigest = orientation, batch, evidence
		RefreshDerived(c); return nil
	} }
	return validation("标本不存在")
}
func RetractSpecimen(c *ProvenanceCase, field string) error {
	if c.Status != ExtractionAuthorized || len(c.Transfers) > 0 { return ErrInvalidTransition }
	for i := range c.Specimens { if c.Specimens[i].FieldNumber == Normalize(field) {
		if c.Specimens[i].Status == "RETRACTED" { return ErrNoChanges }
		c.Specimens[i].Status = "RETRACTED"; c.RetiredSeals = append(c.RetiredSeals, c.Specimens[i].SealCode); sort.Strings(c.RetiredSeals); RefreshDerived(c); return nil
	} }
	return validation("标本不存在")
}

func ValidateTransfer(c *ProvenanceCase, t *CustodyTransfer, actor string, now time.Time) error {
	if c.Status != Extracted && c.Status != InTransit {
		return ErrInvalidTransition
	}
	t.FromActor = Normalize(t.FromActor)
	t.ToActor = Normalize(t.ToActor)
	actor = Normalize(actor)
	if t.FromActor == "" || t.ToActor == "" || t.FromActor == t.ToActor {
		return validation("交出人与接收人必须非空且不同")
	}
	if t.SealStatus != SealIntact && t.SealStatus != SealBroken && t.SealStatus != SealMissing {
		return validation("封签状态无效")
	}
	if actor != t.FromActor {
		return validation("命令操作者必须等于交出人")
	}
	validSeals := map[string]bool{}
	retired := map[string]bool{}
	for _, s := range c.Specimens {
		validSeals[s.SealCode] = true
		for _, h := range s.SealHistory {
			retired[h.OldSealCode] = true
		}
	}
	seen := map[string]bool{}
	for _, seal := range t.AffectedSeals {
		seal = Normalize(seal)
		if seen[seal] {
			return validation("受影响封签不得重复")
		}
		seen[seal] = true
		if retired[seal] && !validSeals[seal] {
			return ErrRetiredSeal
		}
		if !validSeals[seal] {
			return validation("封签不存在")
		}
	}
	if t.SealStatus == SealIntact && len(t.AffectedSeals) > 0 {
		return validation("完好封签不得声明受影响封签")
	}
	if (t.SealStatus == SealBroken || t.SealStatus == SealMissing) && len(t.AffectedSeals) == 0 {
		return validation("异常封签必须指定受影响封签")
	}
	if t.DeclaredCount > uint32(len(c.Specimens)) || t.ReceivedCount > uint32(len(c.Specimens)) {
		return validation("交接数量不得超过标本总数")
	}
	if t.TransferredAt.IsZero() {
		t.TransferredAt = now.UTC()
	}
	if len(c.Transfers) > 0 {
		last := c.Transfers[len(c.Transfers)-1]
		if t.FromActor != last.ToActor {
			return validation("交接责任人不连续")
		}
		if !t.TransferredAt.After(last.TransferredAt) {
			return validation("交接时间必须晚于上一站")
		}
	}
	t.Sequence = uint32(len(c.Transfers) + 1)
	t.Discrepancies = BuildTransferDiscrepancies(c, *t)
	if len(t.Discrepancies) > 0 {
		c.Status = CustodyHold
	} else {
		c.Status = InTransit
	}
	c.Transfers = append(c.Transfers, *t)
	RefreshDerived(c)
	return nil
}
func BuildTransferDiscrepancies(c *ProvenanceCase, t CustodyTransfer) []DiscrepancyItem {
	var out []DiscrepancyItem
	seals := append([]string(nil), t.AffectedSeals...)
	sort.Strings(seals)
	expected := uint32(len(c.Specimens))
	if t.ReceivedCount < t.DeclaredCount {
		out = append(out, discrepancy(t.Sequence, QuantityShort, t.DeclaredCount, t.ReceivedCount, nil, ""))
	}
	if t.ReceivedCount > t.DeclaredCount {
		out = append(out, discrepancy(t.Sequence, QuantityExcess, t.DeclaredCount, t.ReceivedCount, nil, ""))
	}
	if t.DeclaredCount != expected && t.ReceivedCount == t.DeclaredCount {
		kind := QuantityShort
		if t.DeclaredCount > expected {
			kind = QuantityExcess
		}
		out = append(out, discrepancy(t.Sequence, kind, expected, t.DeclaredCount, nil, ""))
	}
	if t.SealStatus != SealIntact && uint32(len(seals)) != t.ReceivedCount {
		if t.ReceivedCount == t.DeclaredCount {
			kind := QuantityShort
			if uint32(len(seals)) > t.ReceivedCount {
				kind = QuantityExcess
			}
			out = append(out, discrepancy(t.Sequence, kind, t.ReceivedCount, uint32(len(seals)), nil, ""))
		}
		out = append(out, discrepancy(t.Sequence, SealCountMismatch, t.ReceivedCount, uint32(len(seals)), seals, t.SealStatus))
	}
	if t.SealStatus == SealBroken {
		out = append(out, discrepancy(t.Sequence, SealDamaged, 0, 0, seals, t.SealStatus))
	}
	if t.SealStatus == SealMissing {
		out = append(out, discrepancy(t.Sequence, SealAbsent, 0, 0, seals, t.SealStatus))
	}
	return out
}
func discrepancy(seq uint32, kind DiscrepancyKind, declared, received uint32, seals []string, status SealStatus) DiscrepancyItem {
	id := fmt.Sprintf("D-%d-%d", seq, kindIndex(kind))
	d := DiscrepancyItem{ID: id, Kind: kind, AffectedSeals: append([]string(nil), seals...), ObservedStatus: status, Status: "OPEN"}
	if kind == QuantityShort || kind == QuantityExcess || kind == SealCountMismatch {
		d.DeclaredCount = uintPtr(declared)
		d.ReceivedCount = uintPtr(received)
	}
	return d
}
func kindIndex(k DiscrepancyKind) uint32 {
	switch k {
	case QuantityShort:
		return 1
	case QuantityExcess:
		return 2
	case SealCountMismatch:
		return 3
	case SealDamaged:
		return 4
	default:
		return 5
	}
}
func ResolveCustody(c *ProvenanceCase, actor string, resolutions map[string]DiscrepancyResolution, now time.Time) ([]string, error) {
	if c.Status != CustodyHold || len(c.Transfers) == 0 {
		return nil, ErrInvalidTransition
	}
	t := &c.Transfers[len(c.Transfers)-1]
	actor = Normalize(actor)
	if actor == "" || actor == t.FromActor {
		return nil, validation("处置人必须独立于异常交接的交出人")
	}
	for i := range t.Discrepancies {
		d := &t.Discrepancies[i]
		r, ok := resolutions[d.ID]
		if !ok {
			continue
		}
		r.ResolutionType = Normalize(r.ResolutionType)
		r.Note = Normalize(r.Note)
		if r.ResolutionType == "" || r.Note == "" {
			return nil, validation("处置类型和说明不得为空")
		}
		closed := false
		if d.Kind == QuantityShort || d.Kind == QuantityExcess || d.Kind == SealCountMismatch {
			closed = r.VerifiedCount != nil && *r.VerifiedCount == *d.DeclaredCount
		}
		if d.Kind == SealDamaged || d.Kind == SealAbsent {
			closed = r.VerifiedStatus == SealIntact
		}
		if !closed {
			continue
		}
		r.ActorID = actor
		r.ResolvedAt = now.UTC()
		d.Resolution = &r
		d.Status = "RESOLVED"
	}
	remaining := []string{}
	for _, d := range t.Discrepancies {
		if d.Status != "RESOLVED" {
			remaining = append(remaining, d.ID)
		}
	}
	if len(remaining) == 0 {
		c.Status = InTransit
		t.ResolutionNote = "全部结构化差异已闭合"
	}
	RefreshDerived(c)
	return remaining, nil
}

func CheckIntake(c *ProvenanceCase, actor string, items []IntakeItem, now time.Time) error {
	if c.Status != InTransit && c.Status != IntakeHold {
		return ErrInvalidTransition
	}
	if len(items) == 0 {
		return validation("逐件验收数组不得为空")
	}
	byField := map[string]int{}
	for i := range c.Specimens {
		byField[c.Specimens[i].FieldNumber] = i
	}
	seenField := map[string]bool{}
	seenSeal := map[string]string{}
	for i := range items {
		item := &items[i]
		item.FieldNumber = Normalize(item.FieldNumber)
		item.ReceivedSealCode = Normalize(item.ReceivedSealCode)
		item.ReceivedCondition = Normalize(item.ReceivedCondition)
		item.EvidenceDigest = Normalize(item.EvidenceDigest)
		idx, ok := byField[item.FieldNumber]
		if !ok {
			return validation(fmt.Sprintf("未知野外编号 %s", item.FieldNumber))
		}
		if seenField[item.FieldNumber] {
			return validation(fmt.Sprintf("重复野外编号 %s", item.FieldNumber))
		}
		seenField[item.FieldNumber] = true
		if other, ok := seenSeal[item.ReceivedSealCode]; ok && item.ReceivedSealCode != "" {
			return validation(fmt.Sprintf("封签 %s 同时对应标本 %s 和 %s", item.ReceivedSealCode, other, item.FieldNumber))
		}
		seenSeal[item.ReceivedSealCode] = item.FieldNumber
		sp := &c.Specimens[idx]
		if c.Status == IntakeHold && sp.IntakeResult == IntakePassed {
			if item.ReceivedSealCode != sp.SealCode || item.EvidenceDigest != sp.EvidenceDigest || item.ReceivedCondition != sp.ReceivedCondition {
				return validation("已通过标本不可覆盖")
			}
		}
		item.Differences = nil
		if item.ReceivedSealCode == "" {
			item.Differences = append(item.Differences, "封签缺失")
		} else if item.ReceivedSealCode != sp.SealCode {
			item.Differences = append(item.Differences, "封签不符")
		}
		if item.ReceivedCondition == "" {
			item.Differences = append(item.Differences, "保存状况缺失")
		} else if strings.EqualFold(item.ReceivedCondition, "BROKEN") || strings.Contains(item.ReceivedCondition, "破损") {
			item.Differences = append(item.Differences, "标本或封签破损")
		}
		if item.EvidenceDigest == "" || item.EvidenceDigest != sp.EvidenceDigest {
			item.Differences = append(item.Differences, "来源摘要不符")
		}
		item.Result = IntakePassed
		if len(item.Differences) > 0 {
			item.Result = IntakeFailed
		}
		sp.ReceivedCondition = item.ReceivedCondition
		sp.IntakeResult = item.Result
		sp.IntakeDifferences = append([]string(nil), item.Differences...)
	}
	for i := range c.Specimens {
		if !seenField[c.Specimens[i].FieldNumber] && (len(c.IntakeHistory) == 0 || c.Specimens[i].IntakeResult == IntakePending) {
			c.Specimens[i].IntakeResult = IntakeFailed
			c.Specimens[i].IntakeDifferences = []string{"未提交验收结果"}
		}
	}
	in := Intake{ReceivedCount: uint32(len(items)), Result: IntakePassed, Actor: Normalize(actor), CheckedAt: now.UTC(), Items: items}
	for _, sp := range c.Specimens {
		if sp.IntakeResult != IntakePassed {
			in.Result = IntakeFailed
			c.Status = IntakeHold
		}
	}
	if in.Result == IntakePassed {
		c.Status = LabAccepted
	}
	if c.Intake != nil {
		in.Resolutions = append([]IntakeResolution(nil), c.Intake.Resolutions...)
	}
	c.Intake = &in
	c.IntakeHistory = append(c.IntakeHistory, in)
	return nil
}
func ResolveIntake(c *ProvenanceCase, actor string, resolutions []IntakeResolution, now time.Time) error {
	if c.Status != IntakeHold || c.Intake == nil {
		return ErrInvalidTransition
	}
	if len(resolutions) == 0 {
		return validation("验收处置不得为空")
	}
	failed := map[string]bool{}
	for _, s := range c.Specimens {
		if s.IntakeResult != IntakePassed {
			failed[s.FieldNumber] = true
		}
	}
	for i := range resolutions {
		r := &resolutions[i]
		r.FieldNumber = Normalize(r.FieldNumber)
		r.Type = Normalize(r.Type)
		r.Note = Normalize(r.Note)
		if !failed[r.FieldNumber] || r.Type == "" || r.Note == "" {
			return validation("处置必须对应失败标本并提供类型和说明")
		}
		r.ActorID = Normalize(actor)
		r.ResolvedAt = now.UTC()
	}
	c.Intake.Resolutions = append(c.Intake.Resolutions, resolutions...)
	return nil
}

func RefreshDerived(c *ProvenanceCase) {
	counts := map[string]uint32{}
	for _, s := range c.Specimens {
		counts[s.ExtractionBatch]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.BatchInventory = c.BatchInventory[:0]
	for _, k := range keys {
		count := counts[k]
		pending := count
		if c.Status == Extracted || c.Status == InTransit || c.Status == CustodyHold || c.Status == IntakeHold || c.Status == LabAccepted || c.Status == Archived {
			pending = 0
		}
		c.BatchInventory = append(c.BatchInventory, BatchInventory{ExtractionBatch: k, RegisteredCount: count, DeclaredCount: count - pending, PendingCount: pending, CanComplete: len(c.Specimens) > 0 && c.Status == ExtractionAuthorized})
	}
	c.CustodyHoldReasons = nil
	byBatch := map[string]*IntakeBatchSummary{}
	for _, s := range c.Specimens {
		x := byBatch[s.ExtractionBatch]
		if x == nil {
			x = &IntakeBatchSummary{ExtractionBatch: s.ExtractionBatch}
			byBatch[s.ExtractionBatch] = x
		}
		switch s.IntakeResult {
		case IntakePassed:
			x.PassedCount++
		case IntakeFailed:
			x.FailedCount++
		default:
			x.PendingCount++
		}
	}
	keys2 := make([]string, 0, len(byBatch))
	for k := range byBatch {
		keys2 = append(keys2, k)
	}
	sort.Strings(keys2)
	c.IntakeBatchSummary = c.IntakeBatchSummary[:0]
	for _, k := range keys2 {
		x := *byBatch[k]
		x.LastRound = len(c.IntakeHistory)
		c.IntakeBatchSummary = append(c.IntakeBatchSummary, x)
	}
	if len(c.Transfers) > 0 {
		last := &c.Transfers[len(c.Transfers)-1]
		c.CurrentCustodian = last.ToActor
		c.NextFromActor = last.ToActor
		t := last.TransferredAt
		c.LastTransferAt = &t
		for _, d := range last.Discrepancies {
			if d.Status != "RESOLVED" {
				c.CustodyHoldReasons = append(c.CustodyHoldReasons, d)
			}
		}
	}
}
func SortedSpecimens(in []SpecimenRecord) []SpecimenRecord {
	out := append([]SpecimenRecord(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].FieldNumber < out[j].FieldNumber })
	return out
}
func timePtr(v time.Time) *time.Time { return &v }
func uintPtr(v uint32) *uint32       { return &v }
