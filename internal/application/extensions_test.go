package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/store"
	"testing"
	"time"
)

func newDraft(t *testing.T, a *App, id string) *domain.ProvenanceCase {
	t.Helper()
	c, err := a.Create(CreateInput{ID: id, SiteName: "地点", StratigraphicUnit: "K1", FieldLead: "field", PermitReference: "P", Latitude: 1, Longitude: 2, DiscoveredAt: time.Now().Add(-time.Hour)}, id+"-create", "field")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestExtendedGovernanceFlow(t *testing.T) {
	s := store.New("")
	a := New(s)
	c := newDraft(t, a, "extended")
	lng := 3.5
	c, err := a.ReviseDraft(c.ID, c.Revision, "revise", "editor", DraftPatch{Longitude: &lng})
	if err != nil || c.Revision != 2 || c.Longitude != lng || c.LastRevisedBy != "editor" {
		t.Fatalf("修订失败: %+v %v", c, err)
	}
	var revisedPayload struct {
		Changes map[string]domain.FieldChange `json:"changes"`
	}
	if err = json.Unmarshal(s.Events(c.ID)[1].PayloadJSON, &revisedPayload); err != nil || revisedPayload.Changes["longitude"].After == nil {
		t.Fatalf("修订事件缺少字段差异: %v", err)
	}

	c, err = a.SubmitReview(c.ID, c.Revision, "review-1", "submitter", domain.Review{ProfileDescription: "剖面一", PhotoDigest: "photo-1", Opinion: "请复核"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.DecideReview(c.ID, c.Revision, "decision-1", "reviewer", false, "照片不清")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.SubmitReview(c.ID, c.Revision, "review-2", "submitter", domain.Review{ProfileDescription: "剖面二", PhotoDigest: "photo-2", Opinion: "已补充"})
	if err != nil {
		t.Fatal(err)
	}
	before := c.Revision
	if _, err = a.DecideReview(c.ID, c.Revision, "bad-reviewer", "submitter", true, "通过"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("应拒绝非独立复核: %v", err)
	}
	unchanged, _ := a.Get(c.ID)
	if unchanged.Revision != before || len(unchanged.ReviewRounds) != 2 {
		t.Fatal("失败决定改变了案件")
	}
	c, err = a.DecideReview(c.ID, c.Revision, "decision-2", "reviewer", true, "证据充分")
	if err != nil || len(c.ReviewRounds) != 2 {
		t.Fatalf("第二轮复核失败: %v", err)
	}

	dup := []domain.SpecimenRecord{{FieldNumber: "F1", Orientation: "N", ExtractionBatch: "B1", EvidenceDigest: "E1"}, {FieldNumber: "F1", Orientation: "S", ExtractionBatch: "B1", EvidenceDigest: "E2"}}
	before = c.Revision
	if _, err = a.AddSpecimens(c.ID, c.Revision, "duplicate", "field", dup); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("应拒绝批内重复编号: %v", err)
	}
	unchanged, _ = a.Get(c.ID)
	if unchanged.Revision != before || len(unchanged.Specimens) != 0 {
		t.Fatal("失败批次发生部分写入")
	}
	items := []domain.SpecimenRecord{{FieldNumber: "F1", Orientation: "N", ExtractionBatch: "B1", EvidenceDigest: "E1", SealCode: "S1"}, {FieldNumber: "F2", Orientation: "S", ExtractionBatch: "B1", EvidenceDigest: "E2", SealCode: "S2"}}
	c, err = a.AddSpecimens(c.ID, c.Revision, "batch", "field", items)
	if err != nil || len(c.Specimens) != 2 || c.BatchInventory[0].RegisteredCount != 2 {
		t.Fatalf("批量登记失败: %v", err)
	}
	c, err = a.ReplaceSeal(c.ID, c.Revision, "seal", "field", "F1", "S1", "S3", "破损")
	if err != nil || c.Specimens[0].SealCode != "S3" || len(c.Specimens[0].SealHistory) != 1 {
		t.Fatalf("封签更换失败: %v", err)
	}
	c, err = a.CompleteExtractionBatches(c.ID, c.Revision, "complete", "field", map[string]uint32{"B1": 2})
	if err != nil || c.Status != domain.Extracted {
		t.Fatalf("清点失败: %v", err)
	}

	c, err = a.Transfer(c.ID, c.Revision, "transfer-1", "field", domain.CustodyTransfer{FromActor: "field", ToActor: "carrier", TransferredAt: time.Now(), DeclaredCount: 2, ReceivedCount: 1, SealStatus: domain.SealBroken, AffectedSeals: []string{"S3"}})
	if err != nil || c.Status != domain.CustodyHold || len(c.Transfers[0].Discrepancies) != 2 {
		t.Fatalf("异常交接失败: %v", err)
	}
	ds := c.Transfers[0].Discrepancies
	count := uint32(2)
	c, remaining, err := a.ResolveCustodyItems(c.ID, c.Revision, "resolve-1", "supervisor", []domain.DiscrepancyResolution{{ResolutionType: "COUNT_RECONCILED", Note: "补齐", VerifiedCount: &count}}, []string{ds[0].ID})
	if err != nil || c.Status != domain.CustodyHold || len(remaining) != 1 {
		t.Fatalf("部分处置结果错误: %v %v", remaining, err)
	}
	c, remaining, err = a.ResolveCustodyItems(c.ID, c.Revision, "resolve-2", "supervisor", []domain.DiscrepancyResolution{{ResolutionType: "RESEALED", Note: "重新封签", VerifiedStatus: domain.SealIntact}}, []string{ds[1].ID})
	if err != nil || len(remaining) != 0 || c.Status != domain.InTransit {
		t.Fatalf("完整处置失败: %v %v", remaining, err)
	}

	c, err = a.IntakeItems(c.ID, c.Revision, "intake-1", "lab", []domain.IntakeItem{{FieldNumber: "F1", ReceivedSealCode: "S3", ReceivedCondition: "完好", EvidenceDigest: "E1"}, {FieldNumber: "F2", ReceivedSealCode: "S2", ReceivedCondition: "完好", EvidenceDigest: "错误"}})
	if err != nil || c.Status != domain.IntakeHold || c.Specimens[1].IntakeResult != domain.IntakeFailed {
		t.Fatalf("逐件差异未识别: %v", err)
	}
	c, err = a.ResolveIntakeItems(c.ID, c.Revision, "intake-resolve", "supervisor", []domain.IntakeResolution{{FieldNumber: "F2", Type: "EVIDENCE_CONFIRMED", Note: "重新核对"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.IntakeItems(c.ID, c.Revision, "intake-2", "lab", []domain.IntakeItem{{FieldNumber: "F2", ReceivedSealCode: "S2", ReceivedCondition: "完好", EvidenceDigest: "E2"}})
	if err != nil || c.Status != domain.LabAccepted || c.Specimens[0].IntakeResult != domain.IntakePassed {
		t.Fatalf("差异闭环失败: %v", err)
	}

	c, err = a.Archive(c.ID, c.Revision, "archive", "archivist")
	if err != nil || c.Status != domain.Archived {
		t.Fatalf("归档失败: %v", err)
	}
	b1, d1, _ := a.Manifest(c.ID)
	b2, d2, _ := a.Manifest(c.ID)
	if !bytes.Equal(b1, b2) || d1 != d2 {
		t.Fatal("重复下载档案不确定")
	}
	page1, err := a.AuditWithCursor(c.ID, "", 2)
	if err != nil || !page1.ChainValid || page1.NextCursor == "" {
		t.Fatalf("第一页审计失败: %v", err)
	}
	page2, err := a.AuditWithCursor(c.ID, page1.NextCursor, 2)
	if err != nil || len(page2.Events) == 0 || page2.Events[0].Revision != page1.Events[len(page1.Events)-1].Revision+1 {
		t.Fatalf("游标续页失败: %v", err)
	}
}

func TestAuditCursorCannotCrossCases(t *testing.T) {
	a := New(store.New(""))
	first := newDraft(t, a, "first")
	lng := 8.0
	if _, err := a.ReviseDraft(first.ID, first.Revision, "first-revise", "editor", DraftPatch{Longitude: &lng}); err != nil {
		t.Fatal(err)
	}
	newDraft(t, a, "second")
	page, err := a.AuditWithCursor("first", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.AuditWithCursor("second", page.NextCursor, 1); err == nil {
		t.Fatal("跨案件游标应被拒绝")
	}
}
