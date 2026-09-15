//nolint:testpackage
package modules

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/scheme"

	. "github.com/onsi/gomega"
)

type forceDeleteMockHandler struct {
	BaseHandler
}

func (m *forceDeleteMockHandler) IsEnabled(_ *PlatformContext) bool {
	return false
}

func (m *forceDeleteMockHandler) BuildModuleCR(_ context.Context, _ client.Client, _ *PlatformContext) (*unstructured.Unstructured, error) {
	return nil, nil
}

func newForceDeleteMockHandler(name string, gvk schema.GroupVersionKind) *forceDeleteMockHandler {
	return &forceDeleteMockHandler{
		BaseHandler: BaseHandler{
			Config: ModuleConfig{
				Name:   name,
				CRName: "default-" + name,
				GVK:    gvk,
			},
		},
	}
}

// withEmptyRegistry swaps the package-level registry for a fresh, empty one so
// this test sees only the handlers it registers, restoring the original
// afterwards. This is only possible from an in-package test (r is unexported).
func withEmptyRegistry(t *testing.T) *Registry {
	t.Helper()

	prev := r
	r = &Registry{}
	t.Cleanup(func() { r = prev })

	return r
}

func newForceDeleteTestClient(t *testing.T, gvks []schema.GroupVersionKind, objs ...client.Object) client.Client {
	t.Helper()

	g := NewWithT(t)

	s, err := scheme.New()
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, gvk := range gvks {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		listGVK := schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"}
		s.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	}

	cli, err := fakeclient.New(fakeclient.WithScheme(s), fakeclient.WithObjects(objs...))
	g.Expect(err).ShouldNot(HaveOccurred())

	return cli
}

func newModuleCR(gvk schema.GroupVersionKind, name string, finalizers []string, deleting bool) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)
	if len(finalizers) > 0 {
		u.SetFinalizers(finalizers)
	}
	if deleting {
		now := metav1.Now()
		u.SetDeletionTimestamp(&now)
	}
	return u
}

func TestForceDeleteAllModuleCRs_EmptyRegistry(t *testing.T) {
	g := NewWithT(t)

	withEmptyRegistry(t)
	cli := newForceDeleteTestClient(t, nil)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_CRAbsent(t *testing.T) {
	g := NewWithT(t)

	testGVK := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "AbsentModule"}
	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("absent-module", testGVK))

	cli := newForceDeleteTestClient(t, []schema.GroupVersionKind{testGVK})

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_NoFinalizers_Deleted(t *testing.T) {
	g := NewWithT(t)

	testGVK := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "PlainModule"}
	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("plain-module", testGVK))

	cr := newModuleCR(testGVK, "default-plain-module", nil, false)
	cli := newForceDeleteTestClient(t, []schema.GroupVersionKind{testGVK}, cr)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(testGVK)
	err := cli.Get(context.Background(), client.ObjectKey{Name: "default-plain-module"}, got)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(client.IgnoreNotFound(err)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_FinalizersNoDeletionTimestamp_ClearedAndDeleted(t *testing.T) {
	g := NewWithT(t)

	testGVK := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "StuckModule"}
	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("stuck-module", testGVK))

	cr := newModuleCR(testGVK, "default-stuck-module", []string{"stuck.io/finalizer"}, false)
	cli := newForceDeleteTestClient(t, []schema.GroupVersionKind{testGVK}, cr)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(testGVK)
	err := cli.Get(context.Background(), client.ObjectKey{Name: "default-stuck-module"}, got)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(client.IgnoreNotFound(err)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_FinalizersAndDeletionTimestamp_Removed(t *testing.T) {
	g := NewWithT(t)

	testGVK := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "TerminatingModule"}
	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("terminating-module", testGVK))

	// Simulates the real scenario: the DSC's foreground GC cascade has already
	// stamped a deletionTimestamp on the CR before we run.
	cr := newModuleCR(testGVK, "default-terminating-module", []string{"external.io/finalizer"}, true)
	cli := newForceDeleteTestClient(t, []schema.GroupVersionKind{testGVK}, cr)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(testGVK)
	err := cli.Get(context.Background(), client.ObjectKey{Name: "default-terminating-module"}, got)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(client.IgnoreNotFound(err)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_GVKNotInScheme_NoOp(t *testing.T) {
	g := NewWithT(t)

	testGVK := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "UnknownModule"}
	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("unknown-module", testGVK))

	// Note: testGVK is intentionally NOT registered on this client's scheme.
	cli := newForceDeleteTestClient(t, nil)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())
}

func TestForceDeleteAllModuleCRs_MultipleModules_OnlyMatchingDeleted(t *testing.T) {
	g := NewWithT(t)

	gvkA := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "ModuleA"}
	gvkB := schema.GroupVersionKind{Group: "test.io", Version: "v1alpha1", Kind: "ModuleB"}

	reg := withEmptyRegistry(t)
	reg.Add(newForceDeleteMockHandler("module-a", gvkA))
	reg.Add(newForceDeleteMockHandler("module-b", gvkB))

	crA := newModuleCR(gvkA, "default-module-a", []string{"a.io/finalizer"}, false)
	// module-b has no CR at all.
	cli := newForceDeleteTestClient(t, []schema.GroupVersionKind{gvkA, gvkB}, crA)

	g.Expect(ForceDeleteAllModuleCRs(context.Background(), cli)).ShouldNot(HaveOccurred())

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(gvkA)
	err := cli.Get(context.Background(), client.ObjectKey{Name: "default-module-a"}, got)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(client.IgnoreNotFound(err)).ShouldNot(HaveOccurred())
}
