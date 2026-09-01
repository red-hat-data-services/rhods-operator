package cluster_test

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	componentApi "github.com/opendatahub-io/opendatahub-operator/v2/api/components/v1alpha1"
	dscv2 "github.com/opendatahub-io/opendatahub-operator/v2/api/datasciencecluster/v2"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"

	. "github.com/onsi/gomega"
)

func TestConfiguredComponentCacheNamespaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		objects     []client.Object
		interceptor *interceptor.Funcs
		expected    []string
	}{
		{
			name:     "includes custom workbench and model-registry namespaces",
			objects:  []client.Object{newDSCWithComponentNamespaces("custom-notebooks", "custom-model-registries")},
			expected: []string{"custom-notebooks", "custom-model-registries"},
		},
		{
			name:     "includes only custom workbench namespace",
			objects:  []client.Object{newDSCWithComponentNamespaces("custom-notebooks", "")},
			expected: []string{"custom-notebooks"},
		},
		{
			name:     "includes only custom model-registry namespace",
			objects:  []client.Object{newDSCWithComponentNamespaces("", "custom-model-registries")},
			expected: []string{"custom-model-registries"},
		},
		{
			name:     "returns empty when both fields are unset",
			objects:  []client.Object{newDSCWithComponentNamespaces("", "")},
			expected: nil,
		},
		{
			name:     "returns empty when DataScienceCluster is missing",
			objects:  []client.Object{},
			expected: nil,
		},
		{
			name:    "returns empty on DataScienceCluster list error",
			objects: []client.Object{},
			interceptor: &interceptor.Funcs{
				List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
					return errors.New("connection refused")
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			opts := []fakeclient.ClientOpts{fakeclient.WithObjects(tt.objects...)}
			if tt.interceptor != nil {
				opts = append(opts, fakeclient.WithInterceptorFuncs(*tt.interceptor))
			}

			cli, err := fakeclient.New(opts...)
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(cluster.ConfiguredComponentCacheNamespaces(t.Context(), cli)).To(Equal(tt.expected))
		})
	}

	t.Run("returns empty when client is nil", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		g.Expect(cluster.ConfiguredComponentCacheNamespaces(t.Context(), nil)).To(BeEmpty())
	})
}

func newDSCWithComponentNamespaces(workbenchNS, registriesNS string) *dscv2.DataScienceCluster {
	return &dscv2.DataScienceCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "default-dsc"},
		Spec: dscv2.DataScienceClusterSpec{
			Components: dscv2.Components{
				Workbenches: componentApi.DSCWorkbenches{
					WorkbenchesCommonSpec: componentApi.WorkbenchesCommonSpec{
						WorkbenchNamespace: workbenchNS,
					},
				},
				ModelRegistry: componentApi.DSCModelRegistry{
					ModelRegistryCommonSpec: componentApi.ModelRegistryCommonSpec{
						RegistriesNamespace: registriesNS,
					},
				},
			},
		},
	}
}
