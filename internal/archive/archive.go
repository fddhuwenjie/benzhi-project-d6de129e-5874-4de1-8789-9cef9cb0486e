package archive

import (
	"encoding/hex"
	"encoding/json"
	"fossil-provenance-ledger/internal/domain"
	"sort"
)

func EventDigest(e domain.AuditEvent) string {
	b, _ := json.Marshal(struct {
		CaseID   string `json:"case_id"`
		Revision uint64 `json:"revision"`
		Type     string `json:"type"`
		Actor    string `json:"actor"`
		Request  string `json:"request"`
		Previous string `json:"previous"`
	}{e.CaseID, e.Revision, e.EventType, e.ActorID, e.RequestID, e.PreviousDigest})
	h := DigestBytes(append(b, e.PayloadJSON...))
	return hex.EncodeToString(h[:])
}

type manifestEvent struct {
	Revision       uint64          `json:"revision"`
	EventType      string          `json:"event_type"`
	ActorID        string          `json:"actor_id"`
	RequestID      string          `json:"request_id"`
	OccurredAt     string          `json:"occurred_at"`
	Payload        json.RawMessage `json:"payload"`
	PreviousDigest string          `json:"previous_digest"`
	Digest         string          `json:"digest"`
}
type manifestSpecimen struct {
	FieldNumber       string                   `json:"field_number"`
	Orientation       string                   `json:"orientation"`
	ExtractionBatch   string                   `json:"extraction_batch"`
	EvidenceDigest    string                   `json:"evidence_digest"`
	SealCode          string                   `json:"seal_code"`
	SealHistory       []domain.SealReplacement `json:"seal_history"`
	ReceivedCondition string                   `json:"received_condition"`
	IntakeResult      domain.IntakeResult      `json:"intake_result"`
	IntakeDifferences []string                 `json:"intake_differences"`
	Status            string                   `json:"status,omitempty"`
}
type manifest struct {
	ID                string                   `json:"id"`
	SiteName          string                   `json:"site_name"`
	StratigraphicUnit string                   `json:"stratigraphic_unit"`
	FieldLead         string                   `json:"field_lead"`
	PermitReference   string                   `json:"permit_reference"`
	Latitude          float64                  `json:"latitude"`
	Longitude         float64                  `json:"longitude"`
	DiscoveredAt      string                   `json:"discovered_at"`
	Status            domain.CaseStatus        `json:"status"`
	Revision          uint64                   `json:"revision"`
	ReviewRounds      []domain.Review          `json:"review_rounds"`
	Specimens         []manifestSpecimen       `json:"specimens"`
	RetiredSeals      []string                 `json:"retired_seals"`
	Transfers         []domain.CustodyTransfer `json:"transfers"`
	IntakeHistory     []domain.Intake          `json:"intake_history"`
	Events            []manifestEvent          `json:"events"`
	FinalEventDigest  string                   `json:"final_event_digest"`
}

func BuildManifest(c *domain.ProvenanceCase, events []domain.AuditEvent) ([]byte, string, error) {
	ss := domain.SortedSpecimens(c.Specimens)
	specimens := make([]manifestSpecimen, 0, len(ss))
	for _, s := range ss {
		specimens = append(specimens, manifestSpecimen{s.FieldNumber, s.Orientation, s.ExtractionBatch, s.EvidenceDigest, s.SealCode, append([]domain.SealReplacement(nil), s.SealHistory...), s.ReceivedCondition, s.IntakeResult, append([]string(nil), s.IntakeDifferences...), s.Status})
	}
	reviews := append([]domain.Review(nil), c.ReviewRounds...)
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].Round < reviews[j].Round })
	transfers := append([]domain.CustodyTransfer(nil), c.Transfers...)
	sort.Slice(transfers, func(i, j int) bool { return transfers[i].Sequence < transfers[j].Sequence })
	stable := StableEvents(events)
	summaries := make([]manifestEvent, 0, len(stable))
	final := ""
	for _, e := range stable {
		payload := json.RawMessage(append([]byte(nil), e.PayloadJSON...))
		if len(payload) == 0 {
			payload = json.RawMessage("null")
		}
		actor, request, digest := e.ActorID, e.RequestID, e.Digest
		occurredAt := e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
		// 归档事件的责任人仍保留在审计链中，但档案摘要使用不含请求上下文的稳定表示，
		// 使预检（尚未知道归档命令 request_id）与实际 manifest 可比较。
		if e.EventType == "CASE_ARCHIVED" {
			actor, request = "", ""
			occurredAt = ""
			canonical := e
			canonical.ActorID, canonical.RequestID = "", ""
			digest = EventDigest(canonical)
		}
		summaries = append(summaries, manifestEvent{Revision: e.Revision, EventType: e.EventType, ActorID: actor, RequestID: request, OccurredAt: occurredAt, Payload: payload, PreviousDigest: e.PreviousDigest, Digest: digest})
		final = digest
	}
	retired := append([]string(nil), c.RetiredSeals...)
	sort.Strings(retired)
	obj := manifest{ID: c.ID, SiteName: c.SiteName, StratigraphicUnit: c.StratigraphicUnit, FieldLead: c.FieldLead, PermitReference: c.PermitReference, Latitude: c.Latitude, Longitude: c.Longitude, DiscoveredAt: c.DiscoveredAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), Status: c.Status, Revision: c.Revision, ReviewRounds: reviews, Specimens: specimens, RetiredSeals: retired, Transfers: transfers, IntakeHistory: append([]domain.Intake(nil), c.IntakeHistory...), Events: summaries, FinalEventDigest: final}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, "", err
	}
	h := DigestBytes(b)
	return b, hex.EncodeToString(h[:]), nil
}
func VerifyChain(events []domain.AuditEvent) bool {
	stable := StableEvents(events)
	prev := ""
	caseID := ""
	for i, e := range stable {
		if i == 0 {
			caseID = e.CaseID
		}
		if e.CaseID != caseID || e.Revision != uint64(i+1) || e.PreviousDigest != prev || EventDigest(e) != e.Digest {
			return false
		}
		prev = e.Digest
	}
	return true
}
func StableEvents(events []domain.AuditEvent) []domain.AuditEvent {
	out := append([]domain.AuditEvent(nil), events...)
	sort.Slice(out, func(i, j int) bool { return out[i].Revision < out[j].Revision })
	return out
}
