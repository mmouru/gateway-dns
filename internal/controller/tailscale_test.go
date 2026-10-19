package controller

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestCreateTailscaleClient(t *testing.T) {
	c := CreateTailscaleClient("example.ts.net", "my-token", nil)

	wantURI := "https://api.tailscale.com/api/v2/tailnet/example.ts.net/dns/split-dns"
	if c.apiURI != wantURI {
		t.Errorf("apiURI = %q, want %q", c.apiURI, wantURI)
	}

	wantBearer := "Bearer my-token"
	if c.bearer != wantBearer {
		t.Errorf("bearer = %q, want %q", c.bearer, wantBearer)
	}

	if c.client == nil {
		t.Error("http.Client is nil")
	}
	if c.client.Timeout == 0 {
		t.Error("http.Client timeout not set")
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

func TestCollapseDomains(t *testing.T) {
	tests := []struct {
		name   string
		routes map[string][]string
		want   map[string][]string
	}{
		{
			name: "siblings with same IPs collapse to parent",
			routes: map[string][]string{
				"app.home.lab":  {"1.2.3.4"},
				"api.home.lab":  {"1.2.3.4"},
			},
			want: map[string][]string{
				"home.lab": {"1.2.3.4"},
			},
		},
		{
			name: "different IPs do not collapse",
			routes: map[string][]string{
				"app.home.lab": {"1.2.3.4"},
				"api.home.lab": {"5.6.7.8"},
			},
			want: map[string][]string{
				"app.home.lab": {"1.2.3.4"},
				"api.home.lab": {"5.6.7.8"},
			},
		},
		{
			name: "single domain stays unchanged",
			routes: map[string][]string{
				"api.home.lab": {"1.2.3.4"},
			},
			want: map[string][]string{
				"api.home.lab": {"1.2.3.4"},
			},
		},
		{
			name: "parent present with same IPs collapses subdomains",
			routes: map[string][]string{
				"api.home.lab": {"1.2.3.4"},
				"home.lab":     {"1.2.3.4"},
			},
			want: map[string][]string{
				"home.lab": {"1.2.3.4"},
			},
		},
		{
			name: "multiple IPs preserved and collapsed",
			routes: map[string][]string{
				"app.home.lab": {"1.2.3.4", "5.6.7.8"},
				"api.home.lab": {"1.2.3.4", "5.6.7.8"},
			},
			want: map[string][]string{
				"home.lab": {"1.2.3.4", "5.6.7.8"},
			},
		},
		{
			name: "mixed domains stay separate",
			routes: map[string][]string{
				"app.home.lab":    {"1.2.3.4"},
				"api.home.lab":    {"1.2.3.4"},
				"other.other.lab": {"1.2.3.4"},
			},
			want: map[string][]string{
				"home.lab":        {"1.2.3.4"},
				"other.other.lab": {"1.2.3.4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseDomains(tt.routes)

			if len(got) != len(tt.want) {
				t.Errorf("collapseDomains() len=%d, want %d; got=%v want=%v", len(got), len(tt.want), got, tt.want)
				return
			}

			for domain, wantIPs := range tt.want {
				gotIPs, ok := got[domain]
				if !ok {
					t.Errorf("missing domain %q in result", domain)
					continue
				}
				if len(gotIPs) != len(wantIPs) {
					t.Errorf("domain %q: got %d IPs, want %d", domain, len(gotIPs), len(wantIPs))
					continue
				}
				for i, ip := range wantIPs {
					if gotIPs[i] != ip {
						t.Errorf("domain %q: ip[%d] = %q, want %q", domain, i, gotIPs[i], ip)
					}
				}
			}
		})
	}
}

func TestNewRequestSetsAuth(t *testing.T) {
	c := CreateTailscaleClient("test.ts.net", "token123", nil)
	req, err := c.newRequest(context.Background(), "GET", nil)
	if err != nil {
		t.Fatalf("newRequest error: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if auth != "Bearer token123" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer token123")
	}
}
