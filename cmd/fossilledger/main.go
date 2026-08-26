package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"fossil-provenance-ledger/internal/application"
	"fossil-provenance-ledger/internal/httpapi"
	"fossil-provenance-ledger/internal/store"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", defaultListenAddress, "监听地址")
	self := flag.Bool("self-check", false, "运行自检")
	flag.Parse()
	if p := os.Getenv("PORT"); p != "" && *addr == "127.0.0.1:19081" {
		*addr = "127.0.0.1:" + p
	}
	*addr = normalizeListenAddress(*addr)
	s := store.New("")
	a := application.New(s)
	api := httpapi.New(a)
	srv := &http.Server{Addr: *addr, Handler: api.Mux}
	if *self {
		if e := runSelfCheck(srv, *addr); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	go func() {
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, e)
		}
	}()
	select {}
}
func post(client *http.Client, base, path string, v any) (map[string]any, error) {
	b, _ := json.Marshal(v)
	r, e := client.Post(base+path, "application/json", bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(r.Body).Decode(&out)
	if r.StatusCode >= 300 {
		return out, fmt.Errorf("status %d: %v", r.StatusCode, out)
	}
	return out, nil
}
func runSelfCheck(srv *http.Server, addr string) error {
	ln, e := netListen(addr)
	if e != nil {
		return e
	}
	srv.Addr = addr
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())
	client := &http.Client{Timeout: 2 * time.Second}
	base := "http://" + addr
	var x map[string]any
	x, e = post(client, base, "/v1/cases", map[string]any{"id": "self-case", "site_name": "测试地点", "stratigraphic_unit": "K1", "field_lead": "field", "permit_reference": "P-1", "latitude": 10, "longitude": 20, "discovered_at": "2024-01-01T00:00:00Z", "request_id": "r1", "actor_id": "field"})
	if e != nil {
		return e
	}
	rev := uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/review", map[string]any{"profile_description": "剖面", "photo_digest": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "opinion": "ok", "expected_revision": rev, "request_id": "r2", "actor_id": "field"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/review-decision", map[string]any{"approve": true, "reason": "来源证据一致", "expected_revision": rev, "request_id": "r3", "actor_id": "reviewer"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/specimens", map[string]any{"field_number": "F1", "orientation": "N", "extraction_batch": "B1", "evidence_digest": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "seal_code": "S1", "expected_revision": rev, "request_id": "r4", "actor_id": "field"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/extraction-complete", map[string]any{"expected_revision": rev, "request_id": "r5", "actor_id": "field"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/transfers", map[string]any{"from_actor": "field", "to_actor": "museum", "declared_count": 1, "received_count": 1, "seal_status": "INTACT", "expected_revision": rev, "request_id": "r6", "actor_id": "field"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	x, e = post(client, base, "/v1/cases/self-case/intake", map[string]any{"received_count": 1, "seal_codes": []string{"S1"}, "expected_revision": rev, "request_id": "r7", "actor_id": "lab"})
	if e != nil {
		return e
	}
	rev = uint64(x["revision"].(float64))
	_, e = post(client, base, "/v1/cases/self-case/archive", map[string]any{"expected_revision": rev, "request_id": "r8", "actor_id": "archivist"})
	return e
}

// netListen is a small indirection to keep self-check setup testable.
var netListen = func(addr string) (net.Listener, error) { return net.Listen("tcp", addr) }
