//nolint:testpackage // Need to test unexported sync/enqueue helpers.
package monitoring

import (
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	serviceApi "github.com/opendatahub-io/opendatahub-operator/v2/api/services/v1alpha1"
	odherrors "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/errors"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

func tempoStackMonitoring(ns, backend string) *serviceApi.Monitoring {
	return &serviceApi.Monitoring{
		ObjectMeta: metav1.ObjectMeta{Name: serviceApi.MonitoringInstanceName},
		Spec: serviceApi.MonitoringSpec{
			MonitoringCommonSpec: serviceApi.MonitoringCommonSpec{
				Namespace: ns,
				Traces: &serviceApi.Traces{
					Storage: serviceApi.TracesStorage{Backend: backend, Secret: "secret"},
				},
			},
		},
	}
}

func gatewayService(ns string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tempoStackGatewayServiceName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/component":  tempoGatewayComponentLabelValue,
				"app.kubernetes.io/managed-by": tempoOperatorManagedByLabelValue,
				"app.kubernetes.io/instance":   tempoStackInstanceLabelValue,
			},
		},
		Spec: corev1.ServiceSpec{Ports: ports},
	}
}

func TestSyncTempoStackGatewayOTLPPort_AddsMissingPort(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	ns := "redhat-ods-monitoring"
	svc := gatewayService(ns, []corev1.ServicePort{
		{Name: "grpc-public", Port: 8090, TargetPort: intstr.FromString("grpc-public")},
		{Name: "public", Port: 8080, TargetPort: intstr.FromString("public")},
	})
	monitoring := tempoStackMonitoring(ns, serviceApi.StorageBackendS3)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, monitoring).Build()
	rr := &odhtypes.ReconciliationRequest{Client: c, Instance: monitoring}

	if err := syncTempoStackGatewayOTLPPort(t.Context(), rr); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := &corev1.Service{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if !tempoStackGatewayHasOTLPPort(got) {
		t.Fatalf("expected otlp-grpc :4317 → grpc-public, got %#v", got.Spec.Ports)
	}
}

func TestSyncTempoStackGatewayOTLPPort_FixesWrongTargetPort(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	ns := "ns"
	svc := gatewayService(ns, []corev1.ServicePort{
		{Name: "otlp-grpc", Port: 4317, TargetPort: intstr.FromInt32(4317)},
		{Name: "grpc-public", Port: 8090, TargetPort: intstr.FromString("grpc-public")},
	})
	monitoring := tempoStackMonitoring(ns, serviceApi.StorageBackendGCS)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, monitoring).Build()
	rr := &odhtypes.ReconciliationRequest{Client: c, Instance: monitoring}

	if err := syncTempoStackGatewayOTLPPort(t.Context(), rr); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := &corev1.Service{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !tempoStackGatewayHasOTLPPort(got) {
		t.Fatalf("expected corrected targetPort grpc-public, got %#v", got.Spec.Ports)
	}
	if len(got.Spec.Ports) != 2 {
		t.Fatalf("should replace wrong 4317 mapping in place, got %#v", got.Spec.Ports)
	}
}

func TestSyncTempoStackGatewayOTLPPort_Idempotent(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	ns := "ns"
	svc := gatewayService(ns, []corev1.ServicePort{
		{Name: "otlp-grpc", Port: 4317, TargetPort: intstr.FromString("grpc-public")},
		{Name: "grpc-public", Port: 8090, TargetPort: intstr.FromString("grpc-public")},
	})
	monitoring := tempoStackMonitoring(ns, serviceApi.StorageBackendGCS)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, monitoring).Build()
	rr := &odhtypes.ReconciliationRequest{Client: c, Instance: monitoring}

	if err := syncTempoStackGatewayOTLPPort(t.Context(), rr); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := &corev1.Service{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Spec.Ports) != 2 {
		t.Fatalf("should not duplicate ports, got %#v", got.Spec.Ports)
	}
}

func TestSyncTempoStackGatewayOTLPPort_SkipsPVBackend(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	monitoring := tempoStackMonitoring("ns", serviceApi.StorageBackendPV)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(monitoring).Build()
	rr := &odhtypes.ReconciliationRequest{Client: c, Instance: monitoring}
	if err := syncTempoStackGatewayOTLPPort(t.Context(), rr); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func TestSyncTempoStackGatewayOTLPPort_RequeuesWhenServiceMissing(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	monitoring := tempoStackMonitoring("ns", serviceApi.StorageBackendS3)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(monitoring).Build()
	rr := &odhtypes.ReconciliationRequest{Client: c, Instance: monitoring}

	err := syncTempoStackGatewayOTLPPort(t.Context(), rr)
	var requeue odherrors.RequeueAfterError
	if !errors.As(err, &requeue) {
		t.Fatalf("want RequeueAfterError, got %v", err)
	}
	if requeue.After != 15*time.Second {
		t.Fatalf("RequeueAfter: want 15s, got %v", requeue.After)
	}
}

func TestIsTempoStackGatewayService(t *testing.T) {
	t.Parallel()

	good := gatewayService("ns", nil)
	if !isTempoStackGatewayService(good) {
		t.Fatal("expected TempoStack gateway Service to match")
	}

	wrongName := good.DeepCopy()
	wrongName.Name = "other"
	if isTempoStackGatewayService(wrongName) {
		t.Fatal("name-only match must not pass")
	}

	missingLabels := good.DeepCopy()
	missingLabels.Labels = map[string]string{"app.kubernetes.io/component": tempoGatewayComponentLabelValue}
	if isTempoStackGatewayService(missingLabels) {
		t.Fatal("incomplete Tempo labels must not match")
	}
}

func TestRequestsForTempoStackGatewayService_FiltersNamespace(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := serviceApi.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	ns := "redhat-ods-monitoring"
	monitoring := tempoStackMonitoring(ns, serviceApi.StorageBackendS3)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(monitoring).Build()

	sameNS := gatewayService(ns, nil)
	if got := requestsForTempoStackGatewayService(t.Context(), c, sameNS); len(got) != 1 || got[0].Name != serviceApi.MonitoringInstanceName {
		t.Fatalf("same-ns gateway should enqueue Monitoring, got %#v", got)
	}

	otherNS := gatewayService("other-ns", nil)
	if got := requestsForTempoStackGatewayService(t.Context(), c, otherNS); len(got) != 0 {
		t.Fatalf("wrong-ns gateway must not enqueue, got %#v", got)
	}

	wrongName := gatewayService(ns, nil)
	wrongName.Name = "other-service"
	if got := requestsForTempoStackGatewayService(t.Context(), c, wrongName); len(got) != 0 {
		t.Fatalf("wrong-name Service must not enqueue, got %#v", got)
	}
}

func TestTracesUseTempoStack_MatchesDeployTracingStack(t *testing.T) {
	t.Parallel()

	if tracesUseTempoStack(tempoStackMonitoring("ns", serviceApi.StorageBackendPV)) {
		t.Fatal("PV must use TempoMonolithic, not TempoStack sync")
	}
	if !tracesUseTempoStack(tempoStackMonitoring("ns", serviceApi.StorageBackendS3)) {
		t.Fatal("S3 must use TempoStack sync")
	}
	if !tracesUseTempoStack(tempoStackMonitoring("ns", serviceApi.StorageBackendGCS)) {
		t.Fatal("GCS must use TempoStack sync")
	}
	if tracesUseTempoStack(&serviceApi.Monitoring{}) {
		t.Fatal("nil traces must skip TempoStack sync")
	}
}
