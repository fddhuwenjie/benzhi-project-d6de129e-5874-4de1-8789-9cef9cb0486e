package persistfailurepartialcommit_test

import (
	"bytes"
	"encoding/json"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/domain"
	"fossil-provenance-ledger/internal/httpapi"
	"fossil-provenance-ledger/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistenceFailureDoesNotPartiallyCommit(t *testing.T) {
	root := t.TempDir()
	var leaked []string

	missingPath := filepath.Join(root, "missing", "ledger.json")
	createStore := store.New(missingPath)
	createServer := httpapi.New(application.New(createStore))
	createBody := []byte(`{"id":"case-create-failure","site_name":"不应创建的地点","stratigraphic_unit":"K1","field_lead":"field","permit_reference":"permit-create","latitude":1,"longitude":2,"discovered_at":"2026-01-02T03:04:05Z","request_id":"create-failure","actor_id":"field"}`)
	create := httptest.NewRequest(http.MethodPost, "/v1/cases", bytes.NewReader(createBody))
	create.Header.Set("Content-Type", "application/json")
	createResult := httptest.NewRecorder()
	createServer.Mux.ServeHTTP(createResult, create)
	if createResult.Code != http.StatusServiceUnavailable {
		t.Fatalf("create persistence failure status = %d, want %d; body=%s", createResult.Code, http.StatusServiceUnavailable, createResult.Body.String())
	}
	failedCreateGet := httptest.NewRequest(http.MethodGet, "/v1/cases/case-create-failure", nil)
	failedCreateResult := httptest.NewRecorder()
	createServer.Mux.ServeHTTP(failedCreateResult, failedCreateGet)
	if failedCreateResult.Code == http.StatusOK {
		leaked = append(leaked, "failed create remains readable")
	}

	liveDir := filepath.Join(root, "live")
	movedDir := filepath.Join(root, "detached")
	if err := os.Mkdir(liveDir, 0o700); err != nil {
		t.Fatal(err)
	}

	livePath := filepath.Join(liveDir, "ledger.json")
	s := store.New(livePath)
	a := application.New(s)
	created, err := a.Create(application.CreateInput{
		ID:                "case-persist",
		SiteName:          "原始地点",
		StratigraphicUnit: "K1",
		FieldLead:         "field",
		PermitReference:   "permit-1",
		Latitude:          1,
		Longitude:         2,
		DiscoveredAt:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, "create-persist", "field")
	if err != nil {
		t.Fatalf("prepare persisted case: %v", err)
	}
	if err := os.Rename(liveDir, movedDir); err != nil {
		t.Fatalf("detach persistence directory: %v", err)
	}

	server := httpapi.New(a)
	body := []byte(`{"request_id":"revise-persist","actor_id":"editor","expected_revision":1,"site_name":"不应生效的地点"}`)
	revise := httptest.NewRequest(http.MethodPatch, "/v1/cases/case-persist", bytes.NewReader(body))
	revise.Header.Set("Content-Type", "application/json")
	reviseResult := httptest.NewRecorder()
	server.Mux.ServeHTTP(reviseResult, revise)
	if reviseResult.Code != http.StatusServiceUnavailable {
		t.Fatalf("persistence failure status = %d, want %d; body=%s", reviseResult.Code, http.StatusServiceUnavailable, reviseResult.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/cases/case-persist", nil)
	getResult := httptest.NewRecorder()
	server.Mux.ServeHTTP(getResult, get)
	if getResult.Code != http.StatusOK {
		t.Fatalf("read live state: status=%d body=%s", getResult.Code, getResult.Body.String())
	}
	var live domain.ProvenanceCase
	if err := json.Unmarshal(getResult.Body.Bytes(), &live); err != nil {
		t.Fatalf("decode live state: %v", err)
	}

	restarted, err := store.New(filepath.Join(movedDir, "ledger.json")).Get(created.ID)
	if err != nil {
		t.Fatalf("reload last durable snapshot: %v", err)
	}
	if live.Revision != restarted.Revision || live.SiteName != restarted.SiteName {
		leaked = append(leaked, "failed update changed live state")
	}
	if len(leaked) != 0 {
		t.Fatalf("persistent write failure leaked partial commits: %s; live revision=%d site=%q, durable revision=%d site=%q", strings.Join(leaked, "; "), live.Revision, live.SiteName, restarted.Revision, restarted.SiteName)
	}
}
