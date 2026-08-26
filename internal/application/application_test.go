package application

import (
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/store"
	"testing"
	"time"
)

func TestFullCaseFlowAndIdempotency(t *testing.T) {
	a := New(store.New(""))
	now := time.Now()
	c, err := a.Create(CreateInput{ID: "c1", SiteName: "地点", StratigraphicUnit: "K1", FieldLead: "field", PermitReference: "P", Latitude: 1, Longitude: 2, DiscoveredAt: now}, "r1", "field")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := a.Create(CreateInput{ID: "c1", SiteName: "地点", StratigraphicUnit: "K1", FieldLead: "field", PermitReference: "P", Latitude: 1, Longitude: 2, DiscoveredAt: now}, "r1", "field")
	if err != nil || replay.Revision != c.Revision {
		t.Fatalf("replay: %v", err)
	}
	c, err = a.SubmitReview("c1", c.Revision, "r2", "field", domain.Review{ProfileDescription: "剖面", PhotoDigest: "sha"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.DecideReview("c1", c.Revision, "r3", "reviewer", true, "通过")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.AddSpecimen("c1", c.Revision, "r4", "field", domain.SpecimenRecord{FieldNumber: "F1", Orientation: "N", ExtractionBatch: "B1", EvidenceDigest: "e"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.CompleteExtraction("c1", c.Revision, "r5", "field")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.Transfer("c1", c.Revision, "r6", "field", domain.CustodyTransfer{FromActor: "field", ToActor: "museum", DeclaredCount: 1, ReceivedCount: 1, SealStatus: domain.SealIntact})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.Intake("c1", c.Revision, "r7", "lab", domain.Intake{ReceivedCount: 1, SealCodes: []string{"SEAL-F1"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.Archive("c1", c.Revision, "r8", "archivist")
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != domain.Archived || c.ArchiveDigest == "" {
		t.Fatalf("not archived: %+v", c)
	}
	if _, err = a.AddSpecimen("c1", c.Revision, "r9", "field", domain.SpecimenRecord{FieldNumber: "F2", Orientation: "N", ExtractionBatch: "B", EvidenceDigest: "e"}); err == nil {
		t.Fatal("expected archived rejection")
	}
}

func TestRevisionConflict(t *testing.T) {
	a := New(store.New(""))
	c, _ := a.Create(CreateInput{ID: "c", SiteName: "x", StratigraphicUnit: "s", FieldLead: "f", PermitReference: "p", DiscoveredAt: time.Now()}, "r", "f")
	if _, err := a.SubmitReview("c", c.Revision-1, "r2", "f", domain.Review{ProfileDescription: "p", PhotoDigest: "d"}); err != store.ErrRevision {
		t.Fatalf("got %v", err)
	}
}
