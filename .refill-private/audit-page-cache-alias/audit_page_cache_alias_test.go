package auditpagecachealias_test

import (
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/store"
	"testing"
	"time"
)

func TestAuditPageCacheResultIsolation(t *testing.T) {
	app := application.New(store.New(""))
	_, err := app.Create(application.CreateInput{
		ID:                "audit-cache-case",
		SiteName:          "野外地点",
		StratigraphicUnit: "K1",
		FieldLead:         "field-lead",
		PermitReference:   "permit-1",
		Latitude:          1,
		Longitude:         2,
		DiscoveredAt:      time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}, "request-audit-cache", "field-lead")
	if err != nil {
		t.Fatal(err)
	}

	first, err := app.AuditWithCursor("audit-cache-case", "", 10)
	if err != nil || len(first.Events) != 1 {
		t.Fatalf("first audit query: events=%d err=%v", len(first.Events), err)
	}
	first.Events[0].EventType = "CALLER_MUTATION"
	first.Events[0].PayloadJSON[0] = '['

	second, err := app.AuditWithCursor("audit-cache-case", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if second.Events[0].EventType != "CASE_CREATED" || string(second.Events[0].PayloadJSON) == string(first.Events[0].PayloadJSON) {
		t.Fatalf("cached audit page leaked caller mutation: type=%q payload=%q", second.Events[0].EventType, second.Events[0].PayloadJSON)
	}
}
