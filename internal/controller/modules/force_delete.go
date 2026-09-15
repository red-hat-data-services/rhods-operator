package modules

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ForceDeleteAllModuleCRs clears finalizers from and deletes every registered
// module's CR, regardless of enabled state, then deletes it. It bypasses the
// module contract entirely — used only by the operator uninstall path
// (pkg/upgrade), where the out-of-tree module operator's own Deployment races
// away in the same Foreground GC cascade that marks the CR for deletion, so
// nothing is ever left running to remove the CR's finalizer through the normal
// module contract.
//
// This is a standalone function, not a ModuleHandler/BaseHandler method: it
// uses only the already-exported GetGVK()/GetName() accessors, so it is not
// part of the module contract.
func ForceDeleteAllModuleCRs(ctx context.Context, cli client.Client) error {
	reg := DefaultRegistry()
	if !reg.HasEntries() {
		return nil
	}

	log := logf.FromContext(ctx)

	return reg.ForAll(func(handler ModuleHandler, _ bool) error {
		gvk := handler.GetGVK()

		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)

		// Module CRs are cluster-scoped; List with no namespace option is the
		// correct, only valid form.
		if err := cli.List(ctx, list); err != nil {
			if meta.IsNoMatchError(err) {
				return nil
			}
			return fmt.Errorf("listing module CRs for %s: %w", handler.GetName(), err)
		}

		for i := range list.Items {
			item := &list.Items[i]

			if len(item.GetFinalizers()) > 0 {
				log.Info("force-clearing finalizers on module CR", "module", handler.GetName(), "name", item.GetName())
				item.SetFinalizers(nil)
				if err := cli.Update(ctx, item); err != nil && !k8serr.IsNotFound(err) {
					return fmt.Errorf("clearing finalizers on %s %s: %w", gvk.Kind, item.GetName(), err)
				}
			}

			if err := cli.Delete(ctx, item); err != nil && !k8serr.IsNotFound(err) {
				return fmt.Errorf("deleting %s %s: %w", gvk.Kind, item.GetName(), err)
			}
		}

		return nil
	})
}
