package dscinitialization

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/scopedcache"
)

const (
	//nolint:gosec // resource name, not a credential
	managedMonitoringSecretName    = "addon-managed-odh-parameters"
	managedMonitoringConfigMapName = "prometheus"
)

func ManagedMonitoringByObject(operatorNs string) map[client.Object]cache.ByObject {
	return map[client.Object]cache.ByObject{
		&corev1.Secret{}: {
			Namespaces: map[string]cache.Config{
				operatorNs: {
					FieldSelector: fields.OneTermEqualSelector("metadata.name", managedMonitoringSecretName),
				},
			},
		},
		&corev1.ConfigMap{}: {
			Namespaces: map[string]cache.Config{
				cluster.DefaultMonitoringNamespaceRHOAI: {
					FieldSelector: fields.OneTermEqualSelector("metadata.name", managedMonitoringConfigMapName),
				},
			},
		},
	}
}

//nolint:ireturn // controller-runtime cache constructors return cache.Cache
func newManagedMonitoringCache(mgr ctrl.Manager, operatorNs string) (cache.Cache, error) {
	return scopedcache.NewForManager(mgr, ManagedMonitoringByObject(operatorNs))
}

//nolint:ireturn // controller-runtime cache constructors return cache.Cache
func managedMonitoringCacheForManager(mgr ctrl.Manager) (cache.Cache, error) {
	operatorNs, err := cluster.GetOperatorNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator namespace: %w", err)
	}

	return newManagedMonitoringCache(mgr, operatorNs)
}
