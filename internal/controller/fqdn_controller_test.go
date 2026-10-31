package controller

import (
	"log/slog"
	"os"
	"testing"

	"github.com/marttimourujarvi/gateway-dns/internal/dnsstore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = gatewayv1.Install(scheme)
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestReconcileGateway_NoTailscaleClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cl := newFakeClient()

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Hostname: addrOfHostname("app.example.com")},
			},
		},
	}

	r := &FQDNReconciler{
		Client:          cl,
		DNSStore:        dnsstore.NewStore(),
		Logger:          logger,
		TailscaleClient: nil,
	}

	_, err := r.reconcileGateway(nil, gw)
	if err != nil {
		t.Fatalf("expected nil error when TailscaleClient is nil, got: %v", err)
	}
}

func TestReconcileGateway_AddSplitDNS(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Hostname: addrOfHostname("app.example.com")},
				{Hostname: addrOfHostname("api.example.com")},
			},
		},
	}
	cl := newFakeClient(gw)

	ts := &TailscaleDNSClient{}
	ts.SetDNSResolverIPs([]string{"10.0.0.1"})

	r := &FQDNReconciler{
		Client:          cl,
		DNSStore:        dnsstore.NewStore(),
		Logger:          logger,
		TailscaleClient: ts,
	}

	_, err := r.reconcileGateway(nil, gw)
	if err != nil {
		t.Fatalf("reconcileGateway add: %v", err)
	}

	// Finalizer should have been set by the controller. We don't have a real
	// client update reflecting back, but we can verify the in-memory gw.
	if len(gw.Finalizers) == 0 {
		t.Error("expected finalizer to be set on gateway")
	}
}

func addrOfHostname(h gatewayv1.Hostname) *gatewayv1.Hostname {
	return &h
}
