package auditcursorgeneration_test

import (
	"bytes"
	"encoding/json"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/httpapi"
	"fossil-provenance-ledger/internal/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestAuditCursorSurvivesUnrelatedWrite(t *testing.T) {
	s := store.New("")
	a := application.New(s)
	discovered := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	c, err := a.Create(application.CreateInput{
		ID: "cursor-case-a", SiteName: "甲地点", StratigraphicUnit: "K1",
		FieldLead: "field-a", PermitReference: "PERMIT-A", Latitude: 10,
		Longitude: 20, DiscoveredAt: discovered,
	}, "create-a", "field-a")
	if err != nil {
		t.Fatal(err)
	}
	longitude := 21.0
	if _, err = a.ReviseDraft(c.ID, c.Revision, "revise-a", "editor-a", application.DraftPatch{Longitude: &longitude}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(a).Mux)
	defer server.Close()

	first := getAuditPage(t, server.URL+"/v1/cases/cursor-case-a/audit?limit=1")
	if first.NextCursor == "" || len(first.Events) != 1 {
		t.Fatalf("第一页没有产生可续页游标: %+v", first)
	}

	createBody := map[string]any{
		"id": "cursor-case-b", "site_name": "乙地点", "stratigraphic_unit": "J2",
		"field_lead": "field-b", "permit_reference": "PERMIT-B", "latitude": 30.0,
		"longitude": 40.0, "discovered_at": discovered.Format(time.RFC3339Nano),
		"request_id": "create-b", "actor_id": "field-b",
	}
	raw, err := json.Marshal(createBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(server.URL+"/v1/cases", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("无关案件建档失败: HTTP %d", resp.StatusCode)
	}

	second := getAuditPage(t, server.URL+"/v1/cases/cursor-case-a/audit?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	if len(second.Events) != 1 || second.Events[0].Revision != 2 {
		t.Fatalf("无关案件写入后未能续查案件 A 的审计页: %+v", second)
	}
}

func getAuditPage(t *testing.T, endpoint string) application.AuditPage {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		t.Fatalf("审计续页返回 HTTP %d: %+v", resp.StatusCode, failure)
	}
	var page application.AuditPage
	if err = json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	return page
}
