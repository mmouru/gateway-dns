package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTailscaleClientCreation(t *testing.T) {
	c := &TailscaleDNSClient{
		client: http.DefaultClient,
		apiURI: "https://api.tailscale.com/api/v2/tailnet/example.ts.net/dns/split-dns",
		bearer: "Bearer my-token",
	}

	wantURI := "https://api.tailscale.com/api/v2/tailnet/example.ts.net/dns/split-dns"
	if c.apiURI != wantURI {
		t.Errorf("apiURI = %q, want %q", c.apiURI, wantURI)
	}

	wantBearer := "Bearer my-token"
	if c.bearer != wantBearer {
		t.Errorf("bearer = %q, want %q", c.bearer, wantBearer)
	}

}

func TestNewBody(t *testing.T) {
	routes := map[string][]string{
		"home.lab": {"10.0.0.1", "10.0.0.2"},
	}

	body, err := newBody(routes)
	if err != nil {
		t.Fatalf("newBody error: %v", err)
	}

	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	var got map[string][]string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 route, got %d", len(got))
	}

	ips := got["home.lab"]
	if len(ips) != 2 {
		t.Errorf("expected 2 IPs, got %d", len(ips))
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"*.example.com", "example.com"},
		{"*.example.local", "example.local"},
		{"example.com", "example.com"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalize(tt.input)
		if got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPatchSplitDNS_NormalizesWildcardDomain(t *testing.T) {
	var patchBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return empty routes on initial GET
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(splitDNSPayload{Routes: map[string][]string{}})
			return
		}
		if r.Method == http.MethodPatch {
			patchBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(splitDNSPayload{Routes: map[string][]string{}})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer ts.Close()

	c := &TailscaleDNSClient{
		client: &http.Client{Timeout: 5 * time.Second},
		apiURI: ts.URL,
		bearer: "Bearer test-token",
	}

	// Pass wildcard domain; normalize should strip *. before storing
	err := c.AddSplitDNS(map[string]string{"*.example.local": "10.0.0.4"})
	if err != nil {
		t.Fatalf("AddSplitDNS error: %v", err)
	}

	var got map[string][]string
	if err := json.Unmarshal(patchBody, &got); err != nil {
		t.Fatalf("unmarshal patch body: %v", err)
	}

	if _, ok := got["*.example.local"]; ok {
		t.Errorf("PATCH body should NOT contain key \"*.example.local\"")
	}

	ips, ok := got["example.local"]
	if !ok {
		t.Fatalf("PATCH body missing normalized key \"example.local\"; got %v", got)
	}
	if len(ips) != 1 || ips[0] != "10.0.0.4" {
		t.Errorf("got %v, want [10.0.0.4]", ips)
	}
}

func TestNewRequestSetsAuth(t *testing.T) {
	c := &TailscaleDNSClient{
		client: http.DefaultClient,
		apiURI: "https://api.tailscale.com/api/v2/tailnet/test.ts.net/dns/split-dns",
		bearer: "Bearer token123",
	}
	req, err := c.newRequest(context.Background(), "GET", nil)
	if err != nil {
		t.Fatalf("newRequest error: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if auth != "Bearer token123" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer token123")
	}
}

