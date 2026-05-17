package plugins

import (
	"testing"

	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestUpdateServiceMonitorNamespaceSelector(t *testing.T) {
	sm := []byte(`
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: test-monitor
  namespace: workload-variant-autoscaler-system
spec:
  namespaceSelector:
    matchNames:
    - workload-variant-autoscaler-system
  selector:
    matchLabels:
      control-plane: controller-manager
`)
	rf := &resource.Factory{}
	res, err := rf.FromBytes(sm)
	if err != nil {
		t.Fatalf("failed to create resource: %v", err)
	}

	rm := resmap.New()
	if err := rm.Append(res); err != nil {
		t.Fatalf("failed to append resource: %v", err)
	}

	if err := UpdateServiceMonitorNamespaceSelector(rm, "opendatahub"); err != nil {
		t.Fatalf("UpdateServiceMonitorNamespaceSelector failed: %v", err)
	}

	node, err := res.Pipe(yaml.Lookup("spec", "namespaceSelector", "matchNames"))
	if err != nil {
		t.Fatalf("failed to lookup matchNames: %v", err)
	}

	elements, err := node.Elements()
	if err != nil {
		t.Fatalf("failed to get elements: %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elements))
	}

	got := elements[0].YNode().Value
	if got != "opendatahub" {
		t.Errorf("expected matchNames[0] = 'opendatahub', got '%s'", got)
	}
}

func TestUpdateServiceMonitorNamespaceSelector_NoMatchNames(t *testing.T) {
	sm := []byte(`
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: test-monitor
spec:
  namespaceSelector:
    any: true
  selector:
    matchLabels:
      control-plane: controller-manager
`)
	rf := &resource.Factory{}
	res, err := rf.FromBytes(sm)
	if err != nil {
		t.Fatalf("failed to create resource: %v", err)
	}

	rm := resmap.New()
	if err := rm.Append(res); err != nil {
		t.Fatalf("failed to append resource: %v", err)
	}

	// Should not error when matchNames is absent
	if err := UpdateServiceMonitorNamespaceSelector(rm, "opendatahub"); err != nil {
		t.Fatalf("UpdateServiceMonitorNamespaceSelector failed: %v", err)
	}
}

func TestUpdateServiceMonitorNamespaceSelector_SkipsNonServiceMonitor(t *testing.T) {
	dep := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: original-ns
spec:
  replicas: 1
`)
	rf := &resource.Factory{}
	res, err := rf.FromBytes(dep)
	if err != nil {
		t.Fatalf("failed to create resource: %v", err)
	}

	rm := resmap.New()
	if err := rm.Append(res); err != nil {
		t.Fatalf("failed to append resource: %v", err)
	}

	// Should not error on non-ServiceMonitor resources
	if err := UpdateServiceMonitorNamespaceSelector(rm, "opendatahub"); err != nil {
		t.Fatalf("UpdateServiceMonitorNamespaceSelector failed: %v", err)
	}
}
