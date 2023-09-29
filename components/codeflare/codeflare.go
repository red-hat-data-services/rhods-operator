// Package codeflare provides utility functions to config CodeFlare as part of the stack which makes managing distributed compute infrastructure in the cloud easy and intuitive for Data Scientists
package codeflare

import (
	"fmt"

	dsci "github.com/opendatahub-io/opendatahub-operator/v2/apis/dscinitialization/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/components"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ComponentName       = "codeflare"
	CodeflarePath       = deploy.DefaultManifestPath + "/" + "codeflare" + "/base"
	CodeflareOperator   = "codeflare-operator"
	RHCodeflareOperator = "rhods-codeflare-operator"
)

var imageParamMap = map[string]string{}

type CodeFlare struct {
	components.Component `json:""`
}

func (d *CodeFlare) SetImageParamsMap(imageMap map[string]string) map[string]string {
	imageParamMap = imageMap
	return imageParamMap
}

func (d *CodeFlare) GetComponentName() string {
	return ComponentName
}

// Verifies that CodeFlare implements ComponentInterface
var _ components.ComponentInterface = (*CodeFlare)(nil)

func (c *CodeFlare) ReconcileComponent(owner metav1.Object, cli client.Client, scheme *runtime.Scheme, managementState operatorv1.ManagementState, dscispec *dsci.DSCInitializationSpec) error {
	enabled := managementState == operatorv1.Managed
	monitoringEnabled := dscispec.Monitoring.ManagementState == operatorv1.Managed

	platform, err := deploy.GetPlatform(cli)
	if err != nil {
		return err
	}

	if enabled {
		// check if the CodeFlare operator is installed
		// codeflare operator not installed
		dependentOperator := CodeflareOperator
		platform, err := deploy.GetPlatform(cli)
		if err != nil {
			return err
		}
		// overwrite dependent operator if downstream not match upstream
		if platform == deploy.SelfManagedRhods || platform == deploy.ManagedRhods {
			dependentOperator = RHCodeflareOperator
		}
		found, err := deploy.OperatorExists(cli, dependentOperator)

		if !found {
			if err != nil {
				return err
			} else {
				return fmt.Errorf("operator %s not found. Please install the operator before enabling %s component",
					dependentOperator, ComponentName)
			}
		}

		// Update image parameters only when we do not have customized manifests set
		if dscispec.DevFlags.ManifestsUri == "" {
			if err := deploy.ApplyImageParams(CodeflarePath, imageParamMap); err != nil {
				return err
			}
		}
	}

	// Deploy Codeflare
	if err := deploy.DeployManifestsFromPath(owner, cli, ComponentName,
		CodeflarePath,
		dscispec.ApplicationsNamespace,
		scheme, enabled); err != nil {
		return err
	}
	// CloudServiceMonitoring handling
	if platform == deploy.ManagedRhods && monitoringEnabled {
		if err := deploy.DeployManifestsFromPath(owner, cli, ComponentName,
			deploy.DefaultManifestPath+"/monitoring/prometheus/rhods/components/"+ComponentName,
			dscispec.Monitoring.Namespace,
			scheme, monitoringEnabled); err != nil {
			return err
		}
	}
	return err
}

func (in *CodeFlare) DeepCopyInto(out *CodeFlare) {
	*out = *in
	out.Component = in.Component
}
