package client_test

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	opclient "github.com/opendatahub-io/opendatahub-operator/v2/pkg/client"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"

	. "github.com/onsi/gomega"
)

// spyClient records what object types are passed to the inner client.
// This is used to verify that the Client wrapper correctly converts
// typed objects to unstructured before calling the inner client,
// ensuring consistent cache usage and preventing the stale informer bug.
type spyClient struct {
	client.Client

	getCalls  []client.Object
	listCalls []client.ObjectList
}

func (s *spyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	s.getCalls = append(s.getCalls, obj)
	return s.Client.Get(ctx, key, obj, opts...)
}

func (s *spyClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	s.listCalls = append(s.listCalls, list)
	return s.Client.List(ctx, list, opts...)
}

func TestClient_Get_Typed(t *testing.T) {
	// Test: Caller passes typed object → use unstructured for cache
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	spy := &spyClient{Client: fakeClient}
	wrappedClient := opclient.New(spy)

	// Get as typed - should convert via unstructured internally
	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("test-cm"))
	g.Expect(result.Namespace).Should(Equal("default"))
	g.Expect(result.Data["key"]).Should(Equal("value"))

	g.Expect(spy.getCalls).Should(HaveLen(1), "expected exactly one call to inner client")

	// Verify the inner client received an unstructured object
	_, isUnstructured := spy.getCalls[0].(*unstructured.Unstructured)
	g.Expect(isUnstructured).Should(BeTrue(),
		"expected unstructured object to be passed to inner client for cache consistency")
}

func TestClient_Get_Unstructured(t *testing.T) {
	// Test: Caller passes unstructured → use unstructured directly (no conversion)
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	spy := &spyClient{Client: fakeClient}

	wrappedClient := opclient.New(spy)

	// Get as unstructured - should use unstructured directly
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(gvk.ConfigMap)
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetName()).Should(Equal("test-cm"))

	g.Expect(spy.getCalls).Should(HaveLen(1))

	// Verify the inner client received the same unstructured object
	passedObj, isUnstructured := spy.getCalls[0].(*unstructured.Unstructured)
	g.Expect(isUnstructured).Should(BeTrue())
	g.Expect(passedObj).Should(Equal(result), "unstructured input should pass through directly")
}

func TestClient_Get_NotFound(t *testing.T) {
	// Test: Error propagation from inner client
	g := NewWithT(t)

	fakeClient, err := fakeclient.New()
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(fakeClient)

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "nonexistent", Namespace: "default"}, result)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("not found"))
}

func TestClient_List_Typed(t *testing.T) {
	// Test: Caller passes typed list → use unstructured for cache
	g := NewWithT(t)

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm-1",
			Namespace: "default",
		},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm-2",
			Namespace: "default",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm1, cm2))
	g.Expect(err).ShouldNot(HaveOccurred())

	spy := &spyClient{Client: fakeClient}
	wrappedClient := opclient.New(spy)

	// List as typed - should convert via unstructured internally
	result := &corev1.ConfigMapList{}
	err = wrappedClient.List(context.Background(), result, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Items).Should(HaveLen(2))
	g.Expect(result.Items[0].GetName()).Should(Equal("test-cm-1"))
	g.Expect(result.Items[1].GetName()).Should(Equal("test-cm-2"))

	g.Expect(spy.listCalls).Should(HaveLen(1), "expected exactly one call to inner client")

	// Verify the inner client received an unstructured list
	_, isUnstructuredList := spy.listCalls[0].(*unstructured.UnstructuredList)
	g.Expect(isUnstructuredList).Should(BeTrue(),
		"expected unstructured list to be passed to inner client for cache consistency")
}

func TestClient_List_Unstructured(t *testing.T) {
	// Test: Caller passes unstructured → use unstructured for cache
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	spy := &spyClient{Client: fakeClient}
	wrappedClient := opclient.New(spy)

	// List as unstructured - should use unstructured directly
	configMapGVK := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMapList",
	}
	result := &unstructured.UnstructuredList{}
	result.SetGroupVersionKind(configMapGVK)
	err = wrappedClient.List(context.Background(), result, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Items).Should(HaveLen(1))
	g.Expect(result.Items[0].GetName()).Should(Equal("test-cm"))

	g.Expect(spy.listCalls).Should(HaveLen(1), "expected exactly one call to inner client")

	// Verify the inner client received an unstructured list
	_, isUnstructuredList := spy.listCalls[0].(*unstructured.UnstructuredList)
	g.Expect(isUnstructuredList).Should(BeTrue(),
		"expected unstructured list to be passed to inner client for cache consistency")
}

func TestClient_Create_Delegates(t *testing.T) {
	g := NewWithT(t)

	fakeClient, err := fakeclient.New()
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(fakeClient)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-cm",
			Namespace: "default",
		},
	}

	err = wrappedClient.Create(context.Background(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Verify it was created
	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "new-cm", Namespace: "default"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("new-cm"))
}

func TestClient_Update_Delegates(t *testing.T) {
	// Test: Update operation delegates to inner client
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "original",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(fakeClient)

	// Get and update
	toUpdate := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, toUpdate)
	g.Expect(err).ShouldNot(HaveOccurred())

	toUpdate.Data["key"] = "updated"
	err = wrappedClient.Update(context.Background(), toUpdate)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Verify update
	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Data["key"]).Should(Equal("updated"))
}

func TestClient_Delete_Delegates(t *testing.T) {
	// Test: Delete operation delegates to inner client
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(fakeClient)

	err = wrappedClient.Delete(context.Background(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Verify deletion
	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("not found"))
}

func TestClient_MetadataPreserved(t *testing.T) {
	// Test: Metadata like ResourceVersion, UID are preserved during conversion
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cm",
			Namespace:       "default",
			UID:             "test-uid-123",
			ResourceVersion: "12345",
			Labels: map[string]string{
				"app": "test",
			},
			Annotations: map[string]string{
				"note": "important",
			},
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	fakeClient, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	// Create wrapped client with NO exemptions (will convert via unstructured)
	wrappedClient := opclient.New(fakeClient)

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(result.UID)).Should(Equal("test-uid-123"))
	g.Expect(result.ResourceVersion).Should(Equal("12345"))
	g.Expect(result.Labels["app"]).Should(Equal("test"))
	g.Expect(result.Annotations["note"]).Should(Equal("important"))
}

func TestClient_Scheme(t *testing.T) {
	g := NewWithT(t)

	fakeClient, err := fakeclient.New()
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(fakeClient)

	g.Expect(wrappedClient.Scheme()).Should(Equal(fakeClient.Scheme()))
}

func TestClient_ImplementsClientInterface(t *testing.T) {
	// Compile-time check that UnstructuredClient implements client.Client
	var _ client.Client = (*opclient.Client)(nil)
}

func TestClient_Get_UnknownNamespaceFallsBackToUncached(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rhods-dashboard-notebooks",
			Namespace: "custom-notebooks",
		},
		Data: map[string]string{"key": "value"},
	}

	cacheErr := fmt.Errorf("unable to get: %s/%s because of unknown namespace for the cache", cm.Namespace, cm.Name)

	cached, err := fakeclient.New(fakeclient.WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return cacheErr
		},
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	uncached, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(cached, opclient.WithUncachedReader(uncached))

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(t.Context(), types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Data["key"]).Should(Equal("value"))
}

func TestClient_Get_UnknownNamespaceWithoutUncachedReader(t *testing.T) {
	g := NewWithT(t)

	cacheErr := fmt.Errorf("unable to get: custom/rhods-dashboard-notebooks because of unknown namespace for the cache")
	cached, err := fakeclient.New(fakeclient.WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return cacheErr
		},
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(cached)

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(t.Context(), types.NamespacedName{Name: "rhods-dashboard-notebooks", Namespace: "custom"}, result)
	g.Expect(err).Should(MatchError(ContainSubstring("unknown namespace for the cache")))
}

func TestClient_List_UnknownNamespaceFallsBackToUncached(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rhods-dashboard-model-registries",
			Namespace: "custom-model-registries",
		},
	}

	cacheErr := fmt.Errorf("unable to list: %s because of unknown namespace for the cache", cm.Namespace)

	cached, err := fakeclient.New(fakeclient.WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
			return cacheErr
		},
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	uncached, err := fakeclient.New(fakeclient.WithObjects(cm))
	g.Expect(err).ShouldNot(HaveOccurred())

	wrappedClient := opclient.New(cached, opclient.WithUncachedReader(uncached))

	list := &corev1.ConfigMapList{}
	err = wrappedClient.List(t.Context(), list, client.InNamespace(cm.Namespace))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(list.Items).To(HaveLen(1))
	g.Expect(list.Items[0].Name).To(Equal(cm.Name))
}
