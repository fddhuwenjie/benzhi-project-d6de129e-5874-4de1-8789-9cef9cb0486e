package case_list_cache_stale_test

import (
	"encoding/json"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/httpapi"
	"fossil-provenance-ledger/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCaseListCacheInvalidatedAfterCreate(t *testing.T) {
	app := application.New(store.New(""))
	createCase(t, app, "case-one", "permit-one")
	server := httpapi.New(app)

	first := listCases(t, server)
	if first.TotalCount != 1 {
		t.Fatalf("首次列表应包含 1 个案件，实际为 %d", first.TotalCount)
	}

	createCase(t, app, "case-two", "permit-two")
	second := listCases(t, server)
	if second.TotalCount != 2 || len(second.Cases) != 2 {
		t.Fatalf("创建第二个案件后列表缓存未失效: total_count=%d cases=%d", second.TotalCount, len(second.Cases))
	}
}

func createCase(t *testing.T, app *application.App, id, permit string) {
	t.Helper()
	_, err := app.Create(application.CreateInput{
		ID: id, SiteName: "测试地点 " + id, StratigraphicUnit: "K1",
		FieldLead: "field-lead", PermitReference: permit,
		Latitude: 10, Longitude: 20, DiscoveredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, "request-"+id, "field-lead")
	if err != nil {
		t.Fatalf("创建案件 %s 失败: %v", id, err)
	}
}

func listCases(t *testing.T, server *httpapi.Server) struct {
	Cases      []json.RawMessage `json:"cases"`
	TotalCount int               `json:"total_count"`
} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/cases?page_size=20", nil)
	recorder := httptest.NewRecorder()
	server.Mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("列表请求返回 HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Cases      []json.RawMessage `json:"cases"`
		TotalCount int               `json:"total_count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("解析列表响应失败: %v", err)
	}
	return response
}
