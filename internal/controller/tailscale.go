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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if tc.logger != nil {
		tc.logger.Debug("tailscale getSplitDNS response", "status", resp.StatusCode, "body", string(body))
	}

	var payload splitDNSPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	return payload.Routes, nil
}

func normalize(host string) string {
	return strings.TrimPrefix(host, "*.")
}

func (tc TailscaleDNSClient) AddSplitDNS(nameserver map[string]string) error {
	_, _, err := tc.patchSplitDNS(nameserver)
	if err != nil {
		fmt.Println("Error during add splitdns: ", err)
	}
	return err
}

func (tc TailscaleDNSClient) DelSplitDNS(domain string) error {
	_, _, err := tc.patchSplitDNS(map[string]string{normalize(domain): ""})
	if err != nil {
		fmt.Println("Error during add splitdns: ", err)
	}
	return err
}

func (tc TailscaleDNSClient) patchSplitDNS(nameserver map[string]string) (bool, string, error) {

	nameservers, err := tc.getSplitDNS()
	if err != nil {
		return false, "", fmt.Errorf("fetch existing split-dns routes: %w", err)
	}
	if nameservers == nil {
		nameservers = make(map[string][]string)
	}

	var d string

	for domain, ns := range nameserver {
		d = normalize(domain)
		if ns == "" {
			nameservers[d] = nil
		} else {
			nameservers[d] = []string{ns}
		}
	}

	bodyJSON, _ := json.Marshal(nameserver)
	if tc.logger != nil {
		tc.logger.Debug("Updated Tailscale splitdns records. a", "body", string(bodyJSON))
	}

	body, err := newBody(nameservers)
	if err != nil {
		return false, d, err
	}
	req, err := tc.newRequest(context.Background(), http.MethodPatch, body)
	if err != nil {
		return false, d, err
	}
	req.Header.Add("Content-type", "application/json")

	resp, err := tc.client.Do(req)
	if err != nil {
		return false, d, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, d, readErr(resp)
	}

	return true, d, nil
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
