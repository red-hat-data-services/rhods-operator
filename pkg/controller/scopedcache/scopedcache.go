package scopedcache

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewForManager creates a dedicated informer cache scoped to specific objects and
// registers it with the manager. Use this for watches that must not affect the
// manager's shared cache (see cert-configmap-generator for the reference pattern).
//
//nolint:ireturn // controller-runtime cache constructors return cache.Cache
func NewForManager(mgr ctrl.Manager, byObject map[client.Object]cache.ByObject) (cache.Cache, error) {
	targetCache, err := cache.New(mgr.GetConfig(), cache.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Scheme:     mgr.GetScheme(),
		Mapper:     mgr.GetRESTMapper(),
		ByObject:   byObject,
		DefaultTransform: func(in any) (any, error) {
			if obj, err := meta.Accessor(in); err == nil && obj.GetManagedFields() != nil {
				obj.SetManagedFields(nil)
			}

			return in, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create scoped cache: %w", err)
	}

	if err := mgr.Add(targetCache); err != nil {
		return nil, fmt.Errorf("unable to register scoped cache with manager: %w", err)
	}

	return targetCache, nil
}
