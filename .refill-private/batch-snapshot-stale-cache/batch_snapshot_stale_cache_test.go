package batchsnapshotstalecache_test

import (
	"errors"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/store"
	"testing"
	"time"
)

func TestStaleBatchSnapshotCannotAuthorizeTransfer(t *testing.T) {
	a := application.New(store.New(""))
	discoveredAt := time.Date(2024, time.May, 12, 9, 0, 0, 0, time.UTC)
	c, err := a.Create(application.CreateInput{
		ID: "stale-snapshot-case", SiteName: "固定剖面", StratigraphicUnit: "K1",
		FieldLead: "field", PermitReference: "PERMIT-1", DiscoveredAt: discoveredAt,
	}, "create-request", "field")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.SubmitReview(c.ID, c.Revision, "review-request", "field", domain.Review{ProfileDescription: "剖面证据", PhotoDigest: "photo-digest"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.DecideReview(c.ID, c.Revision, "decision-request", "reviewer", true, "证据一致")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.AddSpecimen(c.ID, c.Revision, "first-specimen", "field", domain.SpecimenRecord{FieldNumber: "F1", Orientation: "N", ExtractionBatch: "B1", EvidenceDigest: "digest-1"})
	if err != nil {
		t.Fatal(err)
	}

	stale, err := a.BatchSnapshot(c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.AddSpecimen(c.ID, c.Revision, "second-specimen", "field", domain.SpecimenRecord{FieldNumber: "F2", Orientation: "S", ExtractionBatch: "B1", EvidenceDigest: "digest-2"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = a.CompleteExtractionBatches(c.ID, c.Revision, "complete-extraction", "field", map[string]uint32{"B1": 2})
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Transfer(c.ID, c.Revision, "stale-transfer", "field", domain.CustodyTransfer{
		FromActor: "field", ToActor: "museum", DeclaredCount: 2, ReceivedCount: 2,
		SealStatus: domain.SealIntact, SnapshotDigest: stale.Digest,
	})
	if !errors.Is(err, domain.ErrSnapshotConflict) {
		t.Fatalf("stale revision %d snapshot authorized transfer at revision %d: %v", stale.Revision, c.Revision, err)
	}
}
