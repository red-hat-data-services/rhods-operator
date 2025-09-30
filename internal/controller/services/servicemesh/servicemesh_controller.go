package servicemesh

import (
	"context"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	dsciv1 "github.com/opendatahub-io/opendatahub-operator/v2/api/dscinitialization/v1"
	serviceApi "github.com/opendatahub-io/opendatahub-operator/v2/api/services/v1alpha1"
	sr "github.com/opendatahub-io/opendatahub-operator/v2/internal/controller/services/registry"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/deploy"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/render/template"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/reconciler"
)

const (
	ServiceName = "servicemesh"
)

//nolint:gochecknoinits
func init() {
	sr.Add(&serviceHandler{})
}

type serviceHandler struct {
}

func (h *serviceHandler) Init(_ common.Platform) error {
	return nil
}

func (h *serviceHandler) GetName() string {
	return ServiceName
}

func (h *serviceHandler) GetManagementState(_ common.Platform, dsci *dsciv1.DSCInitialization) operatorv1.ManagementState {
	if dsci != nil && dsci.Spec.ServiceMesh != nil {
		return dsci.Spec.ServiceMesh.ManagementState
	}

	return operatorv1.Unmanaged
}

func (h *serviceHandler) NewReconciler(ctx context.Context, mgr ctrl.Manager) error {
	_, err := reconciler.ReconcilerFor(mgr, &serviceApi.ServiceMesh{}).
		WithAction(initialize).
		WithAction(template.NewAction()).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
		)).
		Build(ctx)

	if err != nil {
		return fmt.Errorf("could not create ServiceMesh controller: %w", err)
	}

	return nil
}
