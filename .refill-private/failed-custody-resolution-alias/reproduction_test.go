package failed_custody_resolution_alias_test

import (
	"bytes"
	"encoding/json"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/httpapi"
	"fossil-provenance-ledger/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func custodyHoldCase(t *testing.T, app *application.App) *domain.ProvenanceCase {
	t.Helper()
	c, err := app.Create(application.CreateInput{
		ID: "alias-case", SiteName: "化石坡", StratigraphicUnit: "K1",
		FieldLead: "field", PermitReference: "PERMIT-1", Latitude: 35, Longitude: 105,
		DiscoveredAt: time.Now().UTC().Add(-time.Hour),
	}, "alias-create", "field")
	if err != nil {
		t.Fatal(err)
	}
	c, err = app.SubmitReview(c.ID, c.Revision, "alias-review", "field", domain.Review{
		ProfileDescription: "连续剖面", PhotoDigest: "photo-digest", Opinion: "提交复核",
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err = app.DecideReview(c.ID, c.Revision, "alias-decision", "reviewer", true, "证据完整")
	if err != nil {
		t.Fatal(err)
	}
	c, err = app.AddSpecimen(c.ID, c.Revision, "alias-specimen", "field", domain.SpecimenRecord{
		FieldNumber: "F1", Orientation: "N", ExtractionBatch: "B1", EvidenceDigest: "evidence-1", SealCode: "S1",
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err = app.CompleteExtraction(c.ID, c.Revision, "alias-extraction", "field")
	if err != nil {
		t.Fatal(err)
	}
	c, err = app.Transfer(c.ID, c.Revision, "alias-transfer", "field", domain.CustodyTransfer{
		FromActor: "field", ToActor: "museum", DeclaredCount: 1, ReceivedCount: 0,
		SealStatus: domain.SealBroken, AffectedSeals: []string{"S1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != domain.CustodyHold || len(c.Transfers) != 1 || len(c.Transfers[0].Discrepancies) < 2 {
		t.Fatalf("未建立可复现的交接暂停状态: %+v", c)
	}
	return c
}

func TestFailedCustodyResolutionDoesNotMutateCase(t *testing.T) {
	st := store.New("")
	app := application.New(st)
	c := custodyHoldCase(t, app)
	discrepancies := c.Transfers[0].Discrepancies
	verified := *discrepancies[0].DeclaredCount
	body, err := json.Marshal(map[string]any{
		"request_id": "alias-failed-resolution", "actor_id": "supervisor", "expected_revision": c.Revision,
		"resolutions": []map[string]any{
			{"discrepancy_id": discrepancies[0].ID, "resolution_type": "COUNT_RECONCILED", "note": "已复核", "verified_count": verified},
			{"discrepancy_id": discrepancies[1].ID, "resolution_type": "COUNT_RECONCILED", "note": ""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	api := httpapi.New(app)
	req := httptest.NewRequest(http.MethodPost, "/v1/cases/alias-case/custody-resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.Mux.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("无效处置应返回 HTTP 400，实际为 %d: %s", response.Code, response.Body.String())
	}
	if len(st.Events(c.ID)) != int(c.Revision) {
		t.Fatalf("失败命令不应追加审计事件，实际事件数为 %d", len(st.Events(c.ID)))
	}

	getResponse := httptest.NewRecorder()
	api.Mux.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/v1/cases/alias-case", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("查询案件失败: %d %s", getResponse.Code, getResponse.Body.String())
	}
	var got domain.ProvenanceCase
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Revision != c.Revision {
		t.Fatalf("失败命令不应推进 revision: got %d want %d", got.Revision, c.Revision)
	}
	if got.Transfers[0].Discrepancies[0].Status != "OPEN" || got.Transfers[0].Discrepancies[0].Resolution != nil {
		t.Fatalf("失败处置污染了案件聚合: status=%s resolution=%+v", got.Transfers[0].Discrepancies[0].Status, got.Transfers[0].Discrepancies[0].Resolution)
	}
}
