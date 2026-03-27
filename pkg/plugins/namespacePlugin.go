package plugins

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/builtins" //nolint:staticcheck // Remove after package update
	"sigs.k8s.io/kustomize/api/filters/namespace"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// CreateNamespaceApplierPlugin creates a plugin to ensure resources have the specified target namespace.
func CreateNamespaceApplierPlugin(targetNamespace string) *builtins.NamespaceTransformerPlugin {
	return &builtins.NamespaceTransformerPlugin{
		ObjectMeta: types.ObjectMeta{
			Name:      "odh-namespace-plugin",
			Namespace: targetNamespace,
		},
		FieldSpecs: []types.FieldSpec{
			{
				Gvk:                resid.Gvk{},
				Path:               "metadata/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "rbac.authorization.k8s.io",
					Kind:  "ClusterRoleBinding",
				},
				Path:               "subjects/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "rbac.authorization.k8s.io",
					Kind:  "RoleBinding",
				},
				Path:               "subjects/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "admissionregistration.k8s.io",
					Kind:  "ValidatingWebhookConfiguration",
				},
				Path:               "webhooks/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
			{
				Gvk: resid.Gvk{
					Group: "admissionregistration.k8s.io",
					Kind:  "MutatingWebhookConfiguration",
				},
				Path:               "webhooks/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
			{
				Gvk: resid.Gvk{
					Group: "apiextensions.k8s.io",
					Kind:  "CustomResourceDefinition",
				},
				Path:               "spec/conversion/webhook/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
		},
		UnsetOnly:              false,
		SetRoleBindingSubjects: namespace.AllServiceAccountSubjects,
	}
}

// UpdateServiceMonitorNamespaceSelector updates spec.namespaceSelector.matchNames
// on all ServiceMonitor resources to use the target namespace. The kustomize
// NamespaceTransformerPlugin only handles scalar fields, but matchNames is a
// list of strings, so it must be handled separately.
func UpdateServiceMonitorNamespaceSelector(resMap resmap.ResMap, targetNamespace string) error {
	for _, res := range resMap.Resources() {
		if res.GetKind() != "ServiceMonitor" {
			continue
		}

		node, err := res.Pipe(yaml.Lookup("spec", "namespaceSelector", "matchNames"))
		if err != nil || node == nil {
			continue
		}

		elements, err := node.Elements()
		if err != nil {
			return fmt.Errorf("failed to get matchNames elements on ServiceMonitor %s: %w", res.GetName(), err)
		}

		for _, elem := range elements {
			elem.YNode().Value = targetNamespace
		}
	}
	return nil
}
