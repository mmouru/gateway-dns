package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type splitDNSPayload struct {
	Routes map[string][]string `json:"routes"`
}

func newBody(routes map[string][]string) (*bytes.Reader, error) {
	b, err := json.Marshal(routes)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

const splitDNSURI = "https://api.tailscale.com/api/v2/tailnet/%s/dns/split-dns"

type TailscaleDNSClient struct {
	client       *http.Client
	apiURI       string
	bearer       string
	logger       *slog.Logger
	dnsServerIPs []string
}

func (tc TailscaleDNSClient) DNSResolverIPs() []string {
	return tc.dnsServerIPs
}

func (tc *TailscaleDNSClient) SetDNSResolverIPs(ips []string) {
	tc.dnsServerIPs = ips
}

func (tc TailscaleDNSClient) newRequest(ctx context.Context, method string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, tc.apiURI, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", tc.bearer)
	return req, nil
}

func readErr(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("unexpected status code %d from tailscale api: %s", resp.StatusCode, string(b))
}

func (tc TailscaleDNSClient) getSplitDNS() (map[string][]string, error) {
	req, err := tc.newRequest(context.Background(), http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	resp, err := tc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, readErr(resp)
	}

	var payload splitDNSPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload.Routes, nil
}

func (tc TailscaleDNSClient) patchSplitDNS(routes map[string][]string) (bool, []string, error) {
	routes = collapseDomains(routes)

	existing, err := tc.getSplitDNS()
	if err != nil {
		return false, nil, fmt.Errorf("fetch existing split-dns routes: %w", err)
	}

	merged := make(map[string][]string, len(existing)+len(routes))
	for domain, ips := range existing {
		if ips == nil {
			ips = []string{}
		}
		merged[domain] = ips
	}

	added := make([]string, 0)
	for domain, newIPs := range routes {
		seen := make(map[string]struct{}, len(merged[domain])+len(newIPs))
		for _, ip := range merged[domain] {
			seen[ip] = struct{}{}
		}
		for _, ip := range newIPs {
			if _, ok := seen[ip]; !ok {
				seen[ip] = struct{}{}
				merged[domain] = append(merged[domain], ip)
				added = append(added, fmt.Sprintf("%s -> %s", domain, ip))
			}
		}
	}

	for domain, ips := range merged {
		if ips == nil {
			merged[domain] = []string{}
		}
	}

	bodyJSON, _ := json.Marshal(merged)
	if tc.logger != nil {
		tc.logger.Debug("tailscale patch body", "body", string(bodyJSON))
	}

	body, err := newBody(merged)
	if err != nil {
		return false, added, err
	}
	req, err := tc.newRequest(context.Background(), http.MethodPatch, body)
	if err != nil {
		return false, added, err
	}
	req.Header.Add("Content-type", "application/json")

	resp, err := tc.client.Do(req)
	if err != nil {
		return false, added, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, added, readErr(resp)
	}

	return true, added, nil
}

// NewTailscaleClient checks TAILSCALE_NAMESERVERS, reads the tailscale secret,
// creates a TailscaleDNSClient, and polls for the gateway-dns service LB IP.
// Returns nil when tailscale integration is disabled.
func NewTailscaleClient(reader client.Reader, namespace string, logger *slog.Logger) (*TailscaleDNSClient, error) {
	if !strings.EqualFold(os.Getenv("TAILSCALE_NAMESERVERS"), "true") {
		return nil, nil
	}

	log.Printf("tailscale split-dns sync enabled; reading secret from namespace %s", namespace)

	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: "tailscale", Namespace: namespace}
	if err := reader.Get(context.Background(), key, secret); err != nil {
		return nil, fmt.Errorf("unable to read tailscale secret: %w", err)
	}

	tailnet := string(secret.Data["tailnet"])
	accessToken := string(secret.Data["accessToken"])

	if tailnet == "" || accessToken == "" {
		return nil, fmt.Errorf("tailscale secret missing tailnet or accessToken keys")
	}

	c := &TailscaleDNSClient{
		client: &http.Client{Timeout: 10 * time.Second},
		apiURI: fmt.Sprintf("https://api.tailscale.com/api/v2/tailnet/%s/dns/split-dns", tailnet),
		bearer: "Bearer " + accessToken,
		logger: logger,
	}
	log.Printf("tailscale client initialized for tailnet %s", tailnet)

	var dnsResolverIPs []string
	svcKey := client.ObjectKey{Name: "gateway-dns", Namespace: namespace}
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

pollLoop:
	for {
		svc := &corev1.Service{}
		if err := reader.Get(pollCtx, svcKey, svc); err == nil {
			for _, ing := range svc.Status.LoadBalancer.Ingress {
				if ing.IP != "" {
					dnsResolverIPs = append(dnsResolverIPs, ing.IP)
				}
			}
			if len(dnsResolverIPs) == 0 && len(svc.Spec.ExternalIPs) > 0 {
				dnsResolverIPs = append(dnsResolverIPs, svc.Spec.ExternalIPs...)
			}
			if len(dnsResolverIPs) > 0 {
				log.Printf("DNS resolver IPs for Tailscale: %v", dnsResolverIPs)
				c.SetDNSResolverIPs(dnsResolverIPs)
				break pollLoop
			}
			log.Println("waiting for gateway-dns service LB IP...")
		} else {
			log.Printf("warning: unable to read DNS service gateway-dns/%s: %v", namespace, err)
		}

		select {
		case <-pollCtx.Done():
			log.Println("warning: timed out waiting for gateway-dns service LB IP; Tailscale split-dns sync will be skipped")
			break pollLoop
		case <-time.After(5 * time.Second):
		}
	}

	return c, nil
}

// collapseDomains groups hostnames by their IP sets and replaces sibling
// subdomains with their common parent when all of them share the same IPs.
// For example, { "a.home.lab": ["1.2.3.4"], "b.home.lab": ["1.2.3.4"] }
// becomes { "home.lab": ["1.2.3.4"] }.
func collapseDomains(routes map[string][]string) map[string][]string {
	normalized := make(map[string][]string, len(routes))
	for d, ips := range routes {
		sorted := make([]string, len(ips))
		copy(sorted, ips)
		sort.Strings(sorted)
		normalized[d] = sorted
	}

	result := make(map[string][]string)
	handled := make(map[string]bool)

	var domains []string
	for d := range normalized {
		domains = append(domains, d)
	}
	sort.Slice(domains, func(i, j int) bool {
		return len(domains[i]) > len(domains[j])
	})

	for _, domain := range domains {
		if handled[domain] {
			continue
		}

		parts := strings.Split(domain, ".")
		ips := normalized[domain]
		ipKey := strings.Join(ips, ",")
		collapsed := false

		for strip := 1; strip < len(parts)-1; strip++ {
			parent := strings.Join(parts[strip:], ".")
			if len(strings.Split(parent, ".")) < 2 {
				continue
			}

			var group []string
			for d := range normalized {
				if handled[d] {
					continue
				}
				if d == parent || strings.HasSuffix(d, "."+parent) {
					group = append(group, d)
				}
			}

			if len(group) < 2 {
				continue
			}

			allSame := true
			for _, d := range group {
				if strings.Join(normalized[d], ",") != ipKey {
					allSame = false
					break
				}
			}

			if !allSame {
				continue
			}

			result[parent] = ips
			for _, d := range group {
				handled[d] = true
			}
			collapsed = true
			break
		}

		if !collapsed {
			result[domain] = ips
			handled[domain] = true
		}
	}

	return result
}
