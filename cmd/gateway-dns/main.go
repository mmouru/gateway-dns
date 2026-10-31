package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/marttimourujarvi/gateway-dns/internal/controller"
	"github.com/marttimourujarvi/gateway-dns/internal/dnsstore"
	"github.com/marttimourujarvi/gateway-dns/internal/server"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const PORT = "5353"

func main() {
	fmt.Print(dnsstore.Banner)
	ctrl.SetLogger(zap.New())

	// Single in-memory store shared by the DNS server and the reconciler.
	store := dnsstore.NewStore()

	namespace := getNamespace()

	// Start the DNS server in the background.
	go func() {
		srv := server.New(fmt.Sprintf("0.0.0.0:%s", PORT), store)
		log.Println(fmt.Sprintf("DNS server listening on %s (udp)", PORT))
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("dns server failed: %v", err)
		}
	}()

	// Start the controller-runtime manager (blocks until signalled).
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		log.Fatal(err)
	}

	// Register Gateway API types with the scheme.
	if err := gatewayv1.Install(mgr.GetScheme()); err != nil {
		ctrl.Log.Error(err, "unable to add Gateway API scheme")
		log.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "fqdn-reconciler")

	tsClient, err := controller.NewTailscaleClient(mgr.GetAPIReader(), namespace, logger)
	if err != nil {
		ctrl.Log.Error(err, "unable to initialize tailscale client")
		log.Fatal(err)
	}
	if tsClient == nil {
		log.Println("tailscale split-dns sync disabled (set TAILSCALE_NAMESERVERS=true to enable)")
	}

	if err := (&controller.FQDNReconciler{
		Client:          mgr.GetClient(),
		DNSStore:        store,
		Logger:          logger,
		TailscaleClient: tsClient,
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create controller")
		log.Fatal(err)
	}

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		log.Fatal(err)
	}
}

func getNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "default"
	}
	return strings.TrimSpace(string(data))
}
