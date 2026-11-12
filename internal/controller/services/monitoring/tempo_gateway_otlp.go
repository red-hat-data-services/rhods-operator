package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	serviceApi "github.com/opendatahub-io/opendatahub-operator/v2/api/services/v1alpha1"
	odherrors "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/errors"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

const (
	// tempoStackGatewayServiceName is the Tempo Operator-managed gateway Service
	// for Monitoring's data-science-tempostack instance.
	tempoStackGatewayServiceName     = "tempo-data-science-tempostack-gateway"
	tempoStackInstanceLabelValue     = "data-science-tempostack"
	tempoGatewayComponentLabelValue  = "gateway"
	tempoOperatorManagedByLabelValue = "tempo-operator"
	tempoGatewayOTLPGRPCPortName     = "otlp-grpc"
	tempoGatewayOTLPGRPCPort         = int32(4317)
	tempoGatewayGRPCPublicTargetPort = "grpc-public"
)

// syncTempoStackGatewayOTLPPort ensures the Tempo Operator-managed TempoStack
// gateway Service exposes standard OTLP gRPC :4317 mapped to grpc-public.
// TempoMonolithic already publishes this alias; TempoStack does not.
//
// Deliberate ownership fight: Tempo Operator owns the Service and may strip the
// alias on its reconcile. We re-patch on Monitoring reconcile and on Service
// watch events. Tempo Operator's gateway NetworkPolicy already admits pod
// :8090 (grpc-public), so the Service alias alone is sufficient for collector
// traffic.
//
// Service NotFound (Tempo still rolling out) returns RequeueAfterError so GC
// still runs. Ready may briefly stay True during that wait and during the
// strip→repatch window — RequeueAfter clears provisionErr and the framework
// marks ProvisioningSucceeded=True; we do not hijack TempoAvailable for this.
func syncTempoStackGatewayOTLPPort(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	log := logf.FromContext(ctx).WithName("syncTempoStackGatewayOTLPPort")

	monitoring, ok := rr.Instance.(*serviceApi.Monitoring)
	if !ok {
		return errors.New("instance is not of type *services.Monitoring")
	}
	if !tracesUseTempoStack(monitoring) {
		return nil
	}

	svc := &corev1.Service{}
	key := client.ObjectKey{
		Namespace: monitoring.Spec.Namespace,
		Name:      tempoStackGatewayServiceName,
	}
	if err := rr.Client.Get(ctx, key, svc); err != nil {
		if k8serr.IsNotFound(err) {
			log.Info("Waiting for TempoStack gateway Service before ensuring OTLP port",
				"service", tempoStackGatewayServiceName,
				"namespace", monitoring.Spec.Namespace)
			return odherrors.NewRequeueAfterError(15 * time.Second)
		}
		return fmt.Errorf("get TempoStack gateway Service: %w", err)
	}

	if tempoStackGatewayHasOTLPPort(svc) {
		return nil
	}

	patched := svc.DeepCopy()
	ensureTempoStackGatewayOTLPPort(patched)
	if err := rr.Client.Patch(ctx, patched, client.MergeFrom(svc)); err != nil {
		return fmt.Errorf("ensure OTLP gRPC port %d→%s on TempoStack gateway Service: %w",
			tempoGatewayOTLPGRPCPort, tempoGatewayGRPCPublicTargetPort, err)
	}
	return nil
}

// tracesUseTempoStack mirrors deployTracingStack: any non-PV traces backend
// deploys TempoStack (today S3/GCS); PV deploys TempoMonolithic.
func tracesUseTempoStack(monitoring *serviceApi.Monitoring) bool {
	if monitoring.Spec.Traces == nil {
		return false
	}
	return monitoring.Spec.Traces.Storage.Backend != serviceApi.StorageBackendPV
}

func tempoStackGatewayHasOTLPPort(svc *corev1.Service) bool {
	for _, port := range svc.Spec.Ports {
		if port.Port == tempoGatewayOTLPGRPCPort &&
			port.TargetPort == intstr.FromString(tempoGatewayGRPCPublicTargetPort) {
			return true
		}
	}
	return false
}

// ensureTempoStackGatewayOTLPPort mutates svc so :4317 targets grpc-public.
// Replaces a wrong 4317 mapping if present; otherwise appends the alias.
func ensureTempoStackGatewayOTLPPort(svc *corev1.Service) {
	desired := corev1.ServicePort{
		Name:       tempoGatewayOTLPGRPCPortName,
		Protocol:   corev1.ProtocolTCP,
		Port:       tempoGatewayOTLPGRPCPort,
		TargetPort: intstr.FromString(tempoGatewayGRPCPublicTargetPort),
	}
	for i := range svc.Spec.Ports {
		if svc.Spec.Ports[i].Port == tempoGatewayOTLPGRPCPort {
			svc.Spec.Ports[i] = desired
			return
		}
	}
	svc.Spec.Ports = append(svc.Spec.Ports, desired)
}

func isTempoStackGatewayService(obj client.Object) bool {
	if obj.GetName() != tempoStackGatewayServiceName {
		return false
	}
	labels := obj.GetLabels()
	return labels["app.kubernetes.io/component"] == tempoGatewayComponentLabelValue &&
		labels["app.kubernetes.io/managed-by"] == tempoOperatorManagedByLabelValue &&
		labels["app.kubernetes.io/instance"] == tempoStackInstanceLabelValue
}

// enqueueTempoStackGatewayService enqueues the Monitoring singleton only for
// the TempoStack gateway Service in Monitoring.Spec.Namespace.
// Also checks the Service name so a missing/changed watch predicate cannot
// enqueue on unrelated Services in that namespace.
func enqueueTempoStackGatewayService(c client.Client) handler.EventHandler {
	return handlers.Fn(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return requestsForTempoStackGatewayService(ctx, c, obj)
	})
}

func requestsForTempoStackGatewayService(ctx context.Context, c client.Client, obj client.Object) []reconcile.Request {
	if obj.GetName() != tempoStackGatewayServiceName {
		return nil
	}
	monitoring := &serviceApi.Monitoring{}
	if err := c.Get(ctx, types.NamespacedName{Name: serviceApi.MonitoringInstanceName}, monitoring); err != nil {
		return nil
	}
	if obj.GetNamespace() != monitoring.Spec.Namespace {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Name: serviceApi.MonitoringInstanceName},
	}}
}
