//nolint:testpackage // This test exercises the unexported uninstall drain helper directly.
package upgrade

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	dscApi "github.com/opendatahub-io/opendatahub-operator/v2/api/datasciencecluster/v3"
	dsciv2 "github.com/opendatahub-io/opendatahub-operator/v2/api/dscinitialization/v2"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"

	. "github.com/onsi/gomega"
)

func TestOperatorUninstallDrainsLLMInferenceServicesBeforeDeletingDSC(t *testing.T) {
	g := NewWithT(t)
	service := newLLMInferenceServiceForUninstall(gvk.LLMInferenceServiceV1Alpha2, "llm")
	service.SetFinalizers([]string{"serving.kserve.io/llmisvc-finalizer"})
	config := newLLMInferenceServiceForUninstall(gvk.LLMInferenceServiceConfigV1Alpha2, "preset")

	dscDeleteAttempted := false
	configDeleteAttempted := false
	cli, err := fakeclient.New(
		fakeclient.WithObjects(
			service,
			config,
			&dscApi.DataScienceCluster{ObjectMeta: metav1.ObjectMeta{Name: "default-dsc"}},
			&dsciv2.DSCInitialization{ObjectMeta: metav1.ObjectMeta{Name: "default-dsci"}},
		),
		fakeclient.WithGVKs(
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceV1Alpha1, Scope: meta.RESTScopeNamespace},
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceV1Alpha2, Scope: meta.RESTScopeNamespace},
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceConfigV1Alpha2, Scope: meta.RESTScopeNamespace},
		),
		fakeclient.WithInterceptorFuncs(interceptor.Funcs{
			DeleteAllOf: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteAllOfOption) error {
				if _, ok := obj.(*dscApi.DataScienceCluster); ok {
					dscDeleteAttempted = true
				}
				return c.DeleteAllOf(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if obj.GetObjectKind().GroupVersionKind() == gvk.LLMInferenceServiceConfigV1Alpha2 {
					configDeleteAttempted = true
				}
				return c.Delete(ctx, obj, opts...)
			},
		}),
	)
	g.Expect(err).NotTo(HaveOccurred())

	err = OperatorUninstall(t.Context(), cli, "")
	g.Expect(err).To(MatchError(ContainSubstring("waiting for LLMInferenceServices to finalize: apps/llm")))
	g.Expect(dscDeleteAttempted).To(BeFalse(), "DSC deletion must wait for LLMInferenceService finalization")
	g.Expect(configDeleteAttempted).To(BeFalse(), "LLMInferenceServiceConfig finalizers must remain authoritative")

	remainingService := &unstructured.Unstructured{}
	remainingService.SetGroupVersionKind(gvk.LLMInferenceServiceV1Alpha2)
	g.Expect(cli.Get(t.Context(), client.ObjectKeyFromObject(service), remainingService)).To(Succeed())
	g.Expect(remainingService.GetDeletionTimestamp().IsZero()).To(BeFalse())
}

func TestDrainLLMInferenceServicesDeletesBothVersionsAndLeavesConfigs(t *testing.T) {
	g := NewWithT(t)
	v1alpha1 := newLLMInferenceServiceForUninstall(gvk.LLMInferenceServiceV1Alpha1, "old")
	v1alpha2 := newLLMInferenceServiceForUninstall(gvk.LLMInferenceServiceV1Alpha2, "current")
	config := newLLMInferenceServiceForUninstall(gvk.LLMInferenceServiceConfigV1Alpha2, "preset")

	cli, err := fakeclient.New(
		fakeclient.WithObjects(v1alpha1, v1alpha2, config),
		fakeclient.WithGVKs(
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceV1Alpha1, Scope: meta.RESTScopeNamespace},
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceV1Alpha2, Scope: meta.RESTScopeNamespace},
			fakeclient.GVKMapping{GVK: gvk.LLMInferenceServiceConfigV1Alpha2, Scope: meta.RESTScopeNamespace},
		),
	)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(drainLLMInferenceServices(t.Context(), cli)).To(Succeed())

	for _, serviceGVK := range []schema.GroupVersionKind{gvk.LLMInferenceServiceV1Alpha1, gvk.LLMInferenceServiceV1Alpha2} {
		services := &unstructured.UnstructuredList{}
		services.SetGroupVersionKind(serviceGVK)
		g.Expect(cli.List(t.Context(), services)).To(Succeed())
		g.Expect(services.Items).To(BeEmpty())
	}

	remainingConfig := &unstructured.Unstructured{}
	remainingConfig.SetGroupVersionKind(gvk.LLMInferenceServiceConfigV1Alpha2)
	g.Expect(cli.Get(t.Context(), client.ObjectKeyFromObject(config), remainingConfig)).To(Succeed())
}

func newLLMInferenceServiceForUninstall(gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	resource := &unstructured.Unstructured{}
	resource.SetGroupVersionKind(gvk)
	resource.SetNamespace("apps")
	resource.SetName(name)
	return resource
}
