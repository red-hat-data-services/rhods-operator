package main

import (
	"maps"

	klabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	modelregistryctrl "github.com/opendatahub-io/opendatahub-operator/v2/internal/controller/components/modelregistry"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

// withModelRegistryNamespaces retains existing namespace watches and includes
// Model Registry resources in its isolated controller and custom workload namespaces.
// The fallback watches only resources labeled by the Model Registry renderer.
func withModelRegistryNamespaces(namespaces map[string]cache.Config) map[string]cache.Config {
	result := maps.Clone(namespaces)
	result[cache.AllNamespaces] = cache.Config{
		LabelSelector: klabels.SelectorFromSet(map[string]string{
			labels.ODH.Component(modelregistryctrl.LegacyComponentName): labels.True,
		}),
	}
	return result
}
