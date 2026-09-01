//nolint:testpackage
package dashboard

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	componentApi "github.com/opendatahub-io/opendatahub-operator/v2/api/components/v1alpha1"
	dsciv2 "github.com/opendatahub-io/opendatahub-operator/v2/api/dscinitialization/v2"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"

	. "github.com/onsi/gomega"
)

func newDSCI(appNS string) *dsciv2.DSCInitialization {
	return &dsciv2.DSCInitialization{
		ObjectMeta: metav1.ObjectMeta{Name: "default-dsci"},
		Spec: dsciv2.DSCInitializationSpec{
			ApplicationsNamespace: appNS,
		},
	}
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func newWorkbenches(ns string) *componentApi.Workbenches {
	return &componentApi.Workbenches{
		ObjectMeta: metav1.ObjectMeta{Name: componentApi.WorkbenchesInstanceName},
		Spec: componentApi.WorkbenchesSpec{
			WorkbenchesCommonSpec: componentApi.WorkbenchesCommonSpec{
				WorkbenchNamespace: ns,
			},
		},
	}
}

func newModelRegistry(ns string) *componentApi.ModelRegistry {
	return &componentApi.ModelRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: componentApi.ModelRegistryInstanceName},
		Spec: componentApi.ModelRegistrySpec{
			ModelRegistryCommonSpec: componentApi.ModelRegistryCommonSpec{
				RegistriesNamespace: ns,
			},
		},
	}
}

func TestEnsureNamespacedRBAC(t *testing.T) {
	tests := []struct {
		name               string
		platform           common.Platform
		objects            []client.Object
		expectedCount      int
		expectedRoleNames  []string
		expectedNamespaces map[string]string
	}{
		{
			name:     "both components enabled with namespaces present",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newNamespace("rhods-notebooks"),
				newNamespace("rhoai-model-registries"),
				newWorkbenches("rhods-notebooks"),
				newModelRegistry("rhoai-model-registries"),
			},
			expectedCount:     4,
			expectedRoleNames: []string{"rhods-dashboard-notebooks", "rhods-dashboard-model-registries"},
			expectedNamespaces: map[string]string{
				"rhods-dashboard-notebooks":        "rhods-notebooks",
				"rhods-dashboard-model-registries": "rhoai-model-registries",
			},
		},
		{
			name:     "custom workbench and model-registry namespaces",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newNamespace("custom-notebooks"),
				newNamespace("custom-model-registries"),
				newWorkbenches("custom-notebooks"),
				newModelRegistry("custom-model-registries"),
			},
			expectedCount:     4,
			expectedRoleNames: []string{"rhods-dashboard-notebooks", "rhods-dashboard-model-registries"},
			expectedNamespaces: map[string]string{
				"rhods-dashboard-notebooks":        "custom-notebooks",
				"rhods-dashboard-model-registries": "custom-model-registries",
			},
		},
		{
			name:     "only workbenches enabled",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newNamespace("rhods-notebooks"),
				newWorkbenches("rhods-notebooks"),
			},
			expectedCount:     2,
			expectedRoleNames: []string{"rhods-dashboard-notebooks"},
		},
		{
			name:     "only model-registry enabled",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newNamespace("rhoai-model-registries"),
				newModelRegistry("rhoai-model-registries"),
			},
			expectedCount:     2,
			expectedRoleNames: []string{"rhods-dashboard-model-registries"},
		},
		{
			name:     "neither component enabled",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
			},
			expectedCount: 0,
		},
		{
			name:     "ODH platform uses correct SA name",
			platform: cluster.OpenDataHub,
			objects: []client.Object{
				newDSCI("opendatahub"),
				newNamespace("opendatahub"),
				newNamespace("odh-notebooks"),
				newWorkbenches("odh-notebooks"),
			},
			expectedCount:     2,
			expectedRoleNames: []string{"odh-dashboard-notebooks"},
		},
		{
			name:     "workbenches namespace does not exist",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newWorkbenches("rhods-notebooks"),
			},
			expectedCount: 0,
		},
		{
			name:     "model-registry namespace does not exist",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newDSCI("redhat-ods-applications"),
				newNamespace("redhat-ods-applications"),
				newModelRegistry("rhoai-model-registries"),
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			cli, err := fakeclient.New(fakeclient.WithObjects(tt.objects...))
			g.Expect(err).ShouldNot(HaveOccurred())

			rr := &odhtypes.ReconciliationRequest{
				Client:    cli,
				Instance:  &componentApi.Dashboard{},
				Release:   common.Release{Name: tt.platform},
				Resources: []unstructured.Unstructured{},
			}

			err = ensureNamespacedRBAC(ctx, rr)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(rr.Resources).To(HaveLen(tt.expectedCount))

			for _, expectedName := range tt.expectedRoleNames {
				foundRole := false
				foundRoleBinding := false
				for _, res := range rr.Resources {
					if res.GetName() == expectedName {
						if ns, ok := tt.expectedNamespaces[expectedName]; ok {
							g.Expect(res.GetNamespace()).To(Equal(ns), "resource %s/%s is in unexpected namespace", res.GetKind(), res.GetName())
						}
						switch res.GetKind() {
						case "Role":
							foundRole = true
						case "RoleBinding":
							foundRoleBinding = true
						}
					}
				}
				g.Expect(foundRole).To(BeTrue(), "expected Role %q not found", expectedName)
				g.Expect(foundRoleBinding).To(BeTrue(), "expected RoleBinding %q not found", expectedName)
			}
		})
	}
}

func TestEnsureNamespacedRBAC_RoleBindingSubjects(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cli, err := fakeclient.New(fakeclient.WithObjects(
		newDSCI("redhat-ods-applications"),
		newNamespace("redhat-ods-applications"),
		newNamespace("rhods-notebooks"),
		newWorkbenches("rhods-notebooks"),
	))
	g.Expect(err).ShouldNot(HaveOccurred())

	rr := &odhtypes.ReconciliationRequest{
		Client:    cli,
		Instance:  &componentApi.Dashboard{},
		Release:   common.Release{Name: cluster.SelfManagedRhoai},
		Resources: []unstructured.Unstructured{},
	}

	err = ensureNamespacedRBAC(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, res := range rr.Resources {
		if res.GetKind() != "RoleBinding" {
			continue
		}

		g.Expect(res.GetNamespace()).To(Equal("rhods-notebooks"))

		subjects, _, _ := unstructured.NestedSlice(res.Object, "subjects")
		g.Expect(subjects).To(HaveLen(1))

		subject, ok := subjects[0].(map[string]any)
		g.Expect(ok).To(BeTrue(), "subject should be a map")
		g.Expect(subject["kind"]).To(Equal("ServiceAccount"))
		g.Expect(subject["name"]).To(Equal("rhods-dashboard"))
		g.Expect(subject["namespace"]).To(Equal("redhat-ods-applications"))

		roleRef, _, _ := unstructured.NestedMap(res.Object, "roleRef")
		g.Expect(roleRef["kind"]).To(Equal("Role"))
		g.Expect(roleRef["name"]).To(Equal("rhods-dashboard-notebooks"))
	}
}

func TestEnsureNamespacedRBAC_RoleRules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cli, err := fakeclient.New(fakeclient.WithObjects(
		newDSCI("redhat-ods-applications"),
		newNamespace("redhat-ods-applications"),
		newNamespace("rhods-notebooks"),
		newNamespace("rhoai-model-registries"),
		newWorkbenches("rhods-notebooks"),
		newModelRegistry("rhoai-model-registries"),
	))
	g.Expect(err).ShouldNot(HaveOccurred())

	rr := &odhtypes.ReconciliationRequest{
		Client:    cli,
		Instance:  &componentApi.Dashboard{},
		Release:   common.Release{Name: cluster.SelfManagedRhoai},
		Resources: []unstructured.Unstructured{},
	}

	err = ensureNamespacedRBAC(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, res := range rr.Resources {
		if res.GetKind() != "Role" {
			continue
		}

		rules, _, _ := unstructured.NestedSlice(res.Object, "rules")
		g.Expect(rules).NotTo(BeEmpty(), "Role %q should have rules", res.GetName())

		switch res.GetName() {
		case "rhods-dashboard-notebooks":
			g.Expect(res.GetNamespace()).To(Equal("rhods-notebooks"))
			g.Expect(rules).To(HaveLen(3))

			resourceNames := extractResourceNames(rules)
			g.Expect(resourceNames).To(ContainElements("persistentvolumeclaims", "configmaps", "secrets"))

		case "rhods-dashboard-model-registries":
			g.Expect(res.GetNamespace()).To(Equal("rhoai-model-registries"))
			g.Expect(rules).To(HaveLen(2))

			resourceNames := extractResourceNames(rules)
			g.Expect(resourceNames).To(ContainElements("secrets", "configmaps"))
		}
	}
}

func extractResourceNames(rules []any) []string {
	var names []string
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}
		resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
		names = append(names, resources...)
	}
	return names
}

func TestResolveNotebooksNamespace(t *testing.T) {
	tests := []struct {
		name        string
		platform    common.Platform
		objects     []client.Object
		interceptor *interceptor.Funcs
		expectedNS  string
		expectErr   bool
	}{
		{
			name:     "returns custom namespace from Workbenches CR",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newWorkbenches("custom-notebooks"),
				newNamespace("custom-notebooks"),
			},
			expectedNS: "custom-notebooks",
		},
		{
			name:     "falls back to RHOAI default when namespace field is empty",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newWorkbenches(""),
				newNamespace(cluster.DefaultNotebooksNamespaceRHOAI),
			},
			expectedNS: cluster.DefaultNotebooksNamespaceRHOAI,
		},
		{
			name:     "falls back to ODH default when namespace field is empty",
			platform: cluster.OpenDataHub,
			objects: []client.Object{
				newWorkbenches(""),
				newNamespace(cluster.DefaultNotebooksNamespaceODH),
			},
			expectedNS: cluster.DefaultNotebooksNamespaceODH,
		},
		{
			name:       "returns empty when Workbenches CR not found",
			platform:   cluster.SelfManagedRhoai,
			objects:    []client.Object{},
			expectedNS: "",
		},
		{
			name:     "returns empty when namespace does not exist",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newWorkbenches("nonexistent-ns"),
			},
			expectedNS: "",
		},
		{
			name:     "returns error on non-NotFound Get failure",
			platform: cluster.SelfManagedRhoai,
			objects:  []client.Object{},
			interceptor: &interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*componentApi.Workbenches); ok {
						return errors.New("forbidden")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			expectErr: true,
		},
		{
			name:     "returns error on NamespaceExists failure",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newWorkbenches("some-ns"),
			},
			interceptor: &interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if ns, ok := obj.(*corev1.Namespace); ok && ns.Name == "" && key.Name == "some-ns" {
						return errors.New("connection refused")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests { //nolint:dupl
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			opts := []fakeclient.ClientOpts{fakeclient.WithObjects(tt.objects...)}
			if tt.interceptor != nil {
				opts = append(opts, fakeclient.WithInterceptorFuncs(*tt.interceptor))
			}

			cli, err := fakeclient.New(opts...)
			g.Expect(err).ShouldNot(HaveOccurred())

			rr := &odhtypes.ReconciliationRequest{
				Client:  cli,
				Release: common.Release{Name: tt.platform},
			}

			ns, err := resolveNotebooksNamespace(ctx, rr)
			if tt.expectErr {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(ns).To(Equal(tt.expectedNS))
			}
		})
	}
}

func TestResolveModelRegistryNamespace(t *testing.T) {
	tests := []struct {
		name        string
		platform    common.Platform
		objects     []client.Object
		interceptor *interceptor.Funcs
		expectedNS  string
		expectErr   bool
	}{
		{
			name:     "returns namespace from ModelRegistry CR",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newModelRegistry("rhoai-model-registries"),
				newNamespace("rhoai-model-registries"),
			},
			expectedNS: "rhoai-model-registries",
		},
		{
			name:       "returns empty when ModelRegistry CR not found",
			platform:   cluster.SelfManagedRhoai,
			objects:    []client.Object{},
			expectedNS: "",
		},
		{
			name:     "returns empty when registriesNamespace is empty",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newModelRegistry(""),
			},
			expectedNS: "",
		},
		{
			name:     "returns empty when namespace does not exist",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newModelRegistry("nonexistent-ns"),
			},
			expectedNS: "",
		},
		{
			name:     "returns error on non-NotFound Get failure",
			platform: cluster.SelfManagedRhoai,
			objects:  []client.Object{},
			interceptor: &interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*componentApi.ModelRegistry); ok {
						return errors.New("forbidden")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			expectErr: true,
		},
		{
			name:     "returns error on NamespaceExists failure",
			platform: cluster.SelfManagedRhoai,
			objects: []client.Object{
				newModelRegistry("some-ns"),
			},
			interceptor: &interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if ns, ok := obj.(*corev1.Namespace); ok && ns.Name == "" && key.Name == "some-ns" {
						return errors.New("connection refused")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests { //nolint:dupl
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			opts := []fakeclient.ClientOpts{fakeclient.WithObjects(tt.objects...)}
			if tt.interceptor != nil {
				opts = append(opts, fakeclient.WithInterceptorFuncs(*tt.interceptor))
			}

			cli, err := fakeclient.New(opts...)
			g.Expect(err).ShouldNot(HaveOccurred())

			rr := &odhtypes.ReconciliationRequest{
				Client:  cli,
				Release: common.Release{Name: tt.platform},
			}

			ns, err := resolveModelRegistryNamespace(ctx, rr)
			if tt.expectErr {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(ns).To(Equal(tt.expectedNS))
			}
		})
	}
}

func TestNotebooksRBACRules(t *testing.T) {
	g := NewWithT(t)
	rules := notebooksRBACRules()

	g.Expect(rules).To(HaveLen(3))
	g.Expect(rules[0].Resources).To(ConsistOf("persistentvolumeclaims"))
	g.Expect(rules[0].Verbs).To(ConsistOf("create", "get"))
	g.Expect(rules[1].Resources).To(ConsistOf("configmaps"))
	g.Expect(rules[1].Verbs).To(ConsistOf("create", "get", "update"))
	g.Expect(rules[2].Resources).To(ConsistOf("secrets"))
	g.Expect(rules[2].Verbs).To(ConsistOf("create", "get", "update"))
}

func TestModelRegistryRBACRules(t *testing.T) {
	g := NewWithT(t)
	rules := modelRegistryRBACRules()

	g.Expect(rules).To(HaveLen(2))
	g.Expect(rules[0].Resources).To(ConsistOf("secrets"))
	g.Expect(rules[0].Verbs).To(ConsistOf("create", "delete", "get", "list", "patch"))
	g.Expect(rules[1].Resources).To(ConsistOf("configmaps"))
	g.Expect(rules[1].Verbs).To(ConsistOf("create", "list"))
}
