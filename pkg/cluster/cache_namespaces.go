package cluster

import (
	"context"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ConfiguredComponentCacheNamespaces returns the workbench and model-registry
// namespaces configured on the DataScienceCluster, if one exists.
//
// Dashboard ensureNamespacedRBAC (and other operands) Get/Apply Role and
// RoleBinding objects in these namespaces. The operator informer cache is
// built at manager start and must include them; otherwise controller-runtime
// rejects the cached GET with "unknown namespace for the cache" and the
// Dashboard / ModelRegistry components stay Not Ready.
//
// Returns an empty slice when no DSC exists or the fields are unset. Errors
// other than NotFound are logged and treated as empty so operator startup
// still succeeds with platform defaults (an uncached client is the fallback
// for namespaces that are not in the cache).
func ConfiguredComponentCacheNamespaces(ctx context.Context, cli client.Reader) []string {
	if cli == nil {
		return nil
	}

	dsc, err := GetDSC(ctx, cli)
	if err != nil {
		if !k8serr.IsNotFound(err) {
			logf.FromContext(ctx).Error(err, "unable to read DataScienceCluster for cache namespaces; using defaults only")
		}
		return nil
	}

	var namespaces []string
	if ns := dsc.Spec.Components.Workbenches.WorkbenchNamespace; ns != "" {
		namespaces = append(namespaces, ns)
	}
	if ns := dsc.Spec.Components.ModelRegistry.RegistriesNamespace; ns != "" {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}
