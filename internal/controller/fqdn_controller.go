package controller

import (
	"context"
	"log/slog"
	"net"
	"strings"

	"github.com/marttimourujarvi/gateway-dns/internal/dnsstore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// FQDNReconciler watches HTTPRoute, GRPCRoute and Gateway resources and
// keeps a DNSStore in sync with the IPv4 addresses exposed by parent Gateways.
type FQDNReconciler struct {
	client.Client
	DNSStore        *dnsstore.DNSStore
	Logger          *slog.Logger
	TailscaleClient *TailscaleDNSClient
}

const finalizer = "gateway.dns/finalizer"

// Reconcile handles both HTTPRoute and GRPCRoute objects.
// It is triggered for the primary resource (HTTPRoute) and via Watches
// for GRPCRoute and Gateway changes.
func (r *FQDNReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Try HTTPRoute first.
	var httpRoute gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &httpRoute); err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.Logger.Error("failed to get HTTPRoute", "error", err)
			return ctrl.Result{}, err
		}
	} else {
		return r.reconcileRoute(ctx, &httpRoute, httpRoute.Spec.Hostnames, httpRoute.Spec.ParentRefs)
	}

	// Fall back to GRPCRoute.
	var grpcRoute gatewayv1.GRPCRoute
	if err := r.Get(ctx, req.NamespacedName, &grpcRoute); err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.Logger.Error("failed to get GRPCRoute", "error", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	return r.reconcileRoute(ctx, &grpcRoute, grpcRoute.Spec.Hostnames, grpcRoute.Spec.ParentRefs)
}

// reconcileRoute contains the shared logic for maintaining DNS A-records
// derived from a Route's hostnames and its parent Gateway addresses.
func (r *FQDNReconciler) reconcileRoute(ctx context.Context, obj client.Object, hostnames []gatewayv1.Hostname, parentRefs []gatewayv1.ParentReference) (ctrl.Result, error) {
	// Deletion: remove finalizer and clean up DNS entries.
	if !obj.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(obj, finalizer) {
			r.Logger.Info("deleting DNS records for route", "route", obj.GetName(), "namespace", obj.GetNamespace())
			for _, hostname := range hostnames {
				r.Logger.Info("deleted A record", "hostname", hostname)
				r.DNSStore.Delete(string(hostname) + ".")
			}
			controllerutil.RemoveFinalizer(obj, finalizer)
			if err := r.Update(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer is present on creation/update.
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		r.Logger.Info("adding finalizer to route", "route", obj.GetName(), "namespace", obj.GetNamespace())
		controllerutil.AddFinalizer(obj, finalizer)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		// Update triggers a new reconcile; avoid duplicate DNS work.
		return ctrl.Result{}, nil
	}

	var gatewayIPs []string
	seenIPs := make(map[string]struct{})

	// Clear any existing DNS entries for this route's hostnames so that
	// hostname or gateway changes do not leave stale records behind.
	for _, hostname := range hostnames {
		r.DNSStore.Delete(string(hostname) + ".")
	}

	// Re-create entries based on current Gateway addresses.
	for _, parent := range parentRefs {
		ns := obj.GetNamespace()
		if parent.Namespace != nil {
			ns = string(*parent.Namespace)
		}

		var gw gatewayv1.Gateway
		if err := r.Get(ctx, client.ObjectKey{
			Name:      string(parent.Name),
			Namespace: ns,
		}, &gw); err != nil {
			r.Logger.Error("gateway not found", "gateway", string(parent.Name), "namespace", ns, "error", err)
			continue
		}
		if len(gw.Status.Addresses) == 0 {
			r.Logger.Warn("gateway has no addresses in status", "gateway", gw.Name, "namespace", gw.Namespace)
			continue
		}

		r.Logger.Debug("reconciling gateway addresses", "gateway", gw.Name, "namespace", gw.Namespace, "addresses", len(gw.Status.Addresses))
		for _, addr := range gw.Status.Addresses {
			ip := net.ParseIP(strings.TrimSpace(addr.Value))
			if ip == nil || ip.To4() == nil {
				r.Logger.Debug("skipping non-IPv4 address", "address", addr.Value)
				continue
			}
			if _, ok := seenIPs[ip.String()]; !ok {
				seenIPs[ip.String()] = struct{}{}
				gatewayIPs = append(gatewayIPs, ip.String())
			}
			for _, hostname := range hostnames {
				key := string(hostname) + "."
				if old, exists := r.DNSStore.Lookup(key); !exists || old != ip.String() {
					r.Logger.Info("updated DNS A record", "hostname", string(hostname), "ip", ip.String())
				}
				r.DNSStore.Set(key, ip.String())
			}
		}
	}

	if r.TailscaleClient == nil {
		r.Logger.Debug("tailscale client not configured, skipping split-dns sync")
	} else if resolverIPs := r.TailscaleClient.DNSResolverIPs(); len(resolverIPs) == 0 {
		r.Logger.Warn("tailscale client configured but no DNS resolver IPs available, skipping split-dns sync")
	} else {
		routes := make(map[string][]string, len(hostnames))
		for _, hostname := range hostnames {
			routes[string(hostname)] = resolverIPs
		}
		_, added, err := r.TailscaleClient.patchSplitDNS(routes)
		if err != nil {
			r.Logger.Error("failed to patch tailscale split-dns", "error", err)
		} else if len(added) > 0 {
			r.Logger.Info("added tailscale split-dns routes", "added", added, "route", obj.GetName(), "namespace", obj.GetNamespace())
		}
	}

	return ctrl.Result{}, nil
}

// gatewayToRoutes is called whenever a Gateway changes. It returns reconcile
// requests for every HTTPRoute and GRPCRoute that references the Gateway.
func (r *FQDNReconciler) gatewayToRoutes(ctx context.Context, obj client.Object) []reconcile.Request {
	gw, ok := obj.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	// HTTPRoutes referencing this Gateway.
	var httpRoutes gatewayv1.HTTPRouteList
	if err := r.List(ctx, &httpRoutes); err == nil {
		for i := range httpRoutes.Items {
			route := &httpRoutes.Items[i]
			if routeReferencesGateway(route.Spec.ParentRefs, route.GetNamespace(), gw) {
				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(route),
				})
			}
		}
	}

	// GRPCRoutes referencing this Gateway.
	var grpcRoutes gatewayv1.GRPCRouteList
	if err := r.List(ctx, &grpcRoutes); err == nil {
		for i := range grpcRoutes.Items {
			route := &grpcRoutes.Items[i]
			if routeReferencesGateway(route.Spec.ParentRefs, route.GetNamespace(), gw) {
				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(route),
				})
			}
		}
	}

	return requests
}

// routeReferencesGateway checks whether any ParentRef in refs points to gw.
func routeReferencesGateway(refs []gatewayv1.ParentReference, routeNamespace string, gw *gatewayv1.Gateway) bool {
	for _, ref := range refs {
		if string(ref.Name) != gw.Name {
			continue
		}
		ns := routeNamespace
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		if ns == gw.Namespace {
			return true
		}
	}
	return false
}

// SetupWithManager registers the controller with the manager.
// HTTPRoute is the primary resource; GRPCRoute and Gateway are watched
// so that DNS entries are kept in sync when any of them change.
func (r *FQDNReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.HTTPRoute{}).
		Watches(
			&gatewayv1.GRPCRoute{},
			&handler.EnqueueRequestForObject{},
		).
		Watches(
			&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.gatewayToRoutes),
		).
		Complete(r)
}
