// Package trustyai provides utility functions to config TrustyAI, a bias/fairness and explainability toolkit
// +groupName=datasciencecluster.opendatahub.io
package trustyai

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dsciv1 "github.com/opendatahub-io/opendatahub-operator/v2/apis/dscinitialization/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/components"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
)

var (
	ComponentName     = "trustyai"
	ComponentPathName = "trustyai-service-operator"
	PathUpstream      = deploy.DefaultManifestPath + "/" + ComponentPathName + "/overlays/odh"
	PathDownstream    = deploy.DefaultManifestPath + "/" + ComponentPathName + "/overlays/rhoai"
	OverridePath      = ""
)

// Verifies that TrustyAI implements ComponentInterface.
var _ components.ComponentInterface = (*TrustyAI)(nil)

// TrustyAI struct holds the configuration for the TrustyAI component.
// +kubebuilder:object:generate=true
type TrustyAI struct {
	components.Component `json:""`
}

func (t *TrustyAI) OverrideManifests(ctx context.Context, _ cluster.Platform) error {
	// If devflags are set, update default manifests path
	if len(t.DevFlags.Manifests) != 0 {
		manifestConfig := t.DevFlags.Manifests[0]
		if err := deploy.DownloadManifests(ctx, ComponentPathName, manifestConfig); err != nil {
			return err
		}
		// If overlay is defined, update paths
		defaultKustomizePath := "base"
		if manifestConfig.SourcePath != "" {
			defaultKustomizePath = manifestConfig.SourcePath
		}
		OverridePath = filepath.Join(deploy.DefaultManifestPath, ComponentPathName, defaultKustomizePath)
	}
	return nil
}

func (t *TrustyAI) GetComponentName() string {
	return ComponentName
}

func (t *TrustyAI) ReconcileComponent(ctx context.Context, cli client.Client, logger logr.Logger,
	owner metav1.Object, dscispec *dsciv1.DSCInitializationSpec, platform cluster.Platform, _ bool) error {
	var imageParamMap = map[string]string{
		"trustyaiServiceImage":  "RELATED_IMAGE_ODH_TRUSTYAI_SERVICE_IMAGE",
		"trustyaiOperatorImage": "RELATED_IMAGE_ODH_TRUSTYAI_SERVICE_OPERATOR_IMAGE",
	}
	entryPath := map[cluster.Platform]string{
		cluster.SelfManagedRhoai: PathDownstream,
		cluster.ManagedRhoai:     PathDownstream,
		cluster.OpenDataHub:      PathUpstream,
		cluster.Unknown:          PathUpstream,
	}[platform]

	l := t.ConfigComponentLogger(logger, ComponentName, dscispec)

	enabled := t.GetManagementState() == operatorv1.Managed
	monitoringEnabled := dscispec.Monitoring.ManagementState == operatorv1.Managed

	if enabled {
		if t.DevFlags != nil {
			// Download manifests and update paths
			if err := t.OverrideManifests(ctx, platform); err != nil {
				return err
			}
			if OverridePath != "" {
				entryPath = OverridePath
			}
		}
		if (dscispec.DevFlags == nil || dscispec.DevFlags.ManifestsUri == "") && (t.DevFlags == nil || len(t.DevFlags.Manifests) == 0) {
			if err := deploy.ApplyParams(entryPath, imageParamMap); err != nil {
				return fmt.Errorf("failed to update image %s: %w", entryPath, err)
			}
		}
	}
	// Deploy TrustyAI Operator
	if err := deploy.DeployManifestsFromPath(ctx, cli, owner, entryPath, dscispec.ApplicationsNamespace, t.GetComponentName(), enabled); err != nil {
		return err
	}
	l.Info("apply manifests done")

	// Wait for deployment available
	if enabled {
		if err := cluster.WaitForDeploymentAvailable(ctx, cli, ComponentName, dscispec.ApplicationsNamespace, 10, 2); err != nil {
			return fmt.Errorf("deployment for %s is not ready to server: %w", ComponentName, err)
		}
	}

	// CloudService Monitoring handling
	if platform == cluster.ManagedRhoai {
		if err := t.UpdatePrometheusConfig(cli, l, enabled && monitoringEnabled, ComponentName); err != nil {
			return err
		}
		if err := deploy.DeployManifestsFromPath(ctx, cli, owner,
			filepath.Join(deploy.DefaultManifestPath, "monitoring", "prometheus", "apps"),
			dscispec.Monitoring.Namespace,
			"prometheus", true); err != nil {
			return err
		}
		l.Info("updating SRE monitoring done")
	}
	return nil
}