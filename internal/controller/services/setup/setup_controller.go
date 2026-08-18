package setup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/scopedcache"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/upgrade"
)

type SetupControllerReconciler struct {
	client.Client
}

func (r *SetupControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithName("SetupController")
	log.Info("Reconciling setup controller")

	if !upgrade.HasDeleteConfigMap(ctx, r.Client) {
		return ctrl.Result{}, nil
	}

	if err := upgrade.OperatorUninstall(ctx, r.Client, cluster.GetRelease().Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("operator uninstall failed : %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *SetupControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	operatorNs, err := cluster.GetOperatorNamespace()
	if err != nil {
		return fmt.Errorf("failed to get operator namespace: %w", err)
	}

	deleteConfigMapCache, err := scopedcache.NewForManager(mgr, map[client.Object]cache.ByObject{
		&corev1.ConfigMap{}: {
			Namespaces: map[string]cache.Config{
				operatorNs: {},
			},
			Label: labels.Set{upgrade.DeleteConfigMapLabel: "true"}.AsSelector(),
		},
	})
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("setup-controller").
		WatchesRawSource(
			source.TypedKind[client.Object](
				deleteConfigMapCache,
				resources.GvkToPartial(gvk.ConfigMap),
				handlers.Fn(func(_ context.Context, _ client.Object) []reconcile.Request {
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Name:      "uninstall",
							Namespace: operatorNs,
						},
					}}
				}),
				r.filterDeleteConfigMap(operatorNs),
			),
		).
		Complete(r)
}

func (r *SetupControllerReconciler) filterDeleteConfigMap(operatorNs string) predicate.Funcs {
	filter := func(obj client.Object) bool {
		if obj.GetNamespace() != operatorNs {
			return false
		}

		if obj.GetLabels()[upgrade.DeleteConfigMapLabel] != "true" {
			return false
		}

		return true
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return filter(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filter(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
