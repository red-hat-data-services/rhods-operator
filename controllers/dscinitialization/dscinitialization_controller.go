/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package dscinitialization contains controller logic of CRD DSCInitialization.
package dscinitialization

import (
	"context"
	"errors"
	"fmt"
	routev1 "github.com/openshift/api/route/v1"
	"path/filepath"
	"reflect"

	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/go-logr/logr"
	"github.com/opendatahub-io/opendatahub-operator/v2/controllers/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	authv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dscv1 "github.com/opendatahub-io/opendatahub-operator/v2/apis/datasciencecluster/v1"
	dsciv1 "github.com/opendatahub-io/opendatahub-operator/v2/apis/dscinitialization/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/upgrade"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// DSCInitializationReconciler reconciles a DSCInitialization object.
type DSCInitializationReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Log                   logr.Logger
	Recorder              record.EventRecorder
	ApplicationsNamespace string
}

// +kubebuilder:rbac:groups="dscinitialization.opendatahub.io",resources=dscinitializations/status,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups="dscinitialization.opendatahub.io",resources=dscinitializations/finalizers,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups="dscinitialization.opendatahub.io",resources=dscinitializations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="kfdef.apps.kubeflow.org",resources=kfdefs,verbs=get;list;watch;create;update;patch;delete

// Reconcile contains controller logic specific to DSCInitialization instance updates.
func (r *DSCInitializationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { //nolint
	r.Log.Info("Reconciling DSCInitialization.", "DSCInitialization", req.Namespace, "Request.Name", req.Name)

	instance := &dsciv1.DSCInitialization{}
	// First check if instance is being deleted, return
	if instance.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}
	// Second check if default instance not exists, return error
	// TODO: update logic if we support multiple DSCI CR or different name
	defaultDSCI := types.NamespacedName{Name: "default-dsci"}
	err := r.Client.Get(ctx, defaultDSCI, instance)
	if err != nil {
		if apierrs.IsNotFound(err) {
			// DSCInitialization instance not found
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Failed to retrieve DSCInitialization resource.", "DSCInitialization", req.Namespace, "Request.Name", "default-dsci")
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "DSCInitializationReconcileError", "Failed to retrieve DSCInitialization instance default")
		return ctrl.Result{}, err
	}

	// Last check if multiple instances of DSCInitialization, exit with error
	instanceList := &dsciv1.DSCInitializationList{}
	err = r.Client.List(ctx, instanceList)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(instanceList.Items) > 1 {
		message := fmt.Sprintf("only one instance of DSCInitialization object is allowed. Update existing instance on namespace %s and name %s", req.Namespace, req.Name)
		return ctrl.Result{}, errors.New(message)
	}

	// Get platform
	platform, err := deploy.GetPlatform(r.Client)
	if err != nil {
		r.Log.Error(err, "Failed to determine platform (odh vs managed vs self-managed)")
		return reconcile.Result{}, err
	}

	switch req.Name {
	case "prometheus": // prometheus configmap
		if instance.Spec.Monitoring.ManagementState == operatorv1.Managed && platform == deploy.ManagedRhods {
			r.Log.Info("Monitoring enabled to restart deployment", "cluster", "Managed Service Mode")
			err := r.configureManagedMonitoring(ctx, instance, "updates")
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	case "addon-managed-odh-parameters":
		if instance.Spec.Monitoring.ManagementState == operatorv1.Managed && platform == deploy.ManagedRhods {
			r.Log.Info("Monitoring enabled when notification updated", "cluster", "Managed Service Mode")
			err := r.configureManagedMonitoring(ctx, instance, "updates")
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	case "backup": // revert back to the original prometheus.yml
		if instance.Spec.Monitoring.ManagementState == operatorv1.Managed && platform == deploy.ManagedRhods {
			r.Log.Info("Monitoring enabled to restore back", "cluster", "Managed Service Mode")
			err := r.configureManagedMonitoring(ctx, instance, "revertbackup")
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	default:
		// Check namespace is not exist, then create
		namespace := instance.Spec.ApplicationsNamespace
		r.Log.Info("Standard Reconciling workflow to create namespaces")
		err = r.createOdhNamespace(ctx, instance, namespace)
		if err != nil {
			// no need to log error as it was already logged in createOdhNamespace
			return reconcile.Result{}, err
		}

		// Start reconciling
		if instance.Status.Conditions == nil {
			reason := status.ReconcileInit
			message := "Initializing DSCInitialization resource"
			instance, err = r.updateStatus(ctx, instance, func(saved *dsciv1.DSCInitialization) {
				status.SetProgressingCondition(&saved.Status.Conditions, reason, message)
				saved.Status.Phase = status.PhaseProgressing
			})
			if err != nil {
				r.Log.Error(err, "Failed to add conditions to status of DSCInitialization resource.", "DSCInitialization", req.Namespace, "Request.Name", req.Name)
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "DSCInitializationReconcileError",
					"%s for instance %s", message, instance.Name)
				return reconcile.Result{}, err
			}
		}

		switch platform {
		case deploy.SelfManagedRhods:
			err := r.createUserGroup(ctx, instance, "rhods-admins")
			if err != nil {
				return reconcile.Result{}, err
			}
			if instance.Spec.Monitoring.ManagementState == operatorv1.Managed {
				r.Log.Info("Monitoring enabled, won't apply changes", "cluster", "Self-Managed RHODS Mode")
				err = r.configureCommonMonitoring(instance)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
		case deploy.ManagedRhods:
			osdConfigsPath := filepath.Join(deploy.DefaultManifestPath, "osd-configs")
			err = deploy.DeployManifestsFromPath(r.Client, instance, osdConfigsPath, r.ApplicationsNamespace, "osd", true)
			if err != nil {
				r.Log.Error(err, "Failed to apply osd specific configs from manifests", "Manifests path", osdConfigsPath)
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "DSCInitializationReconcileError", "Failed to apply "+osdConfigsPath)
				return reconcile.Result{}, err
			}
			if instance.Spec.Monitoring.ManagementState == operatorv1.Managed {
				r.Log.Info("Monitoring enabled in initialization stage", "cluster", "Managed Service Mode")
				err := r.configureManagedMonitoring(ctx, instance, "init")
				if err != nil {
					return reconcile.Result{}, err
				}
				err = r.configureCommonMonitoring(instance)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
		default:
			err := r.createUserGroup(ctx, instance, "odh-admins")
			if err != nil {
				return reconcile.Result{}, err
			}
			if instance.Spec.Monitoring.ManagementState == operatorv1.Managed {
				r.Log.Info("Monitoring enabled, won't apply changes", "cluster", "ODH Mode")
			}
		}

		// Finish reconciling
		_, err = r.updateStatus(ctx, instance, func(saved *dsciv1.DSCInitialization) {
			status.SetCompleteCondition(&saved.Status.Conditions, status.ReconcileCompleted, status.ReconcileCompletedMessage)
			saved.Status.Phase = status.PhaseReady
		})
		if err != nil {
			r.Log.Error(err, "failed to update DSCInitialization status after successfully completed reconciliation")
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, "DSCInitializationReconcileError", "Failed to update DSCInitialization status")
		}
		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DSCInitializationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// add predicates prevents meaningless reconciliations from being triggered
		// not use WithEventFilter() because it conflict with secret and configmap predicate
		For(&dsciv1.DSCInitialization{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.Namespace{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.Secret{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.ConfigMap{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&netv1.NetworkPolicy{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&authv1.Role{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&authv1.RoleBinding{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&authv1.ClusterRole{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&authv1.ClusterRoleBinding{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&appsv1.ReplicaSet{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.Pod{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&corev1.Service{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Owns(&routev1.Route{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
		Watches(&source.Kind{Type: &dscv1.DataScienceCluster{}}, handler.EnqueueRequestsFromMapFunc(r.watchDSCResrouce), builder.WithPredicates(DSCDeletionPredicate)).
		Watches(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(r.watchMonitoringSecretResrouce), builder.WithPredicates(SecretContentChangedPredicate)).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, handler.EnqueueRequestsFromMapFunc(r.watchMonitoringConfigMapResrouce), builder.WithPredicates(CMContentChangedPredicate)).
		Complete(r)
}

func (r *DSCInitializationReconciler) updateStatus(ctx context.Context, original *dsciv1.DSCInitialization, update func(saved *dsciv1.DSCInitialization),
) (*dsciv1.DSCInitialization, error) {
	saved := &dsciv1.DSCInitialization{}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(original), saved); err != nil {
			return err
		}

		update(saved)

		// Return err itself here (not wrapped inside another error)
		// so that RetryOnConflict can identify it correctly.
		return r.Client.Status().Update(ctx, saved)
	})
	return saved, err
}

var SecretContentChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldSecret := e.ObjectOld.(*corev1.Secret)
		newSecret := e.ObjectNew.(*corev1.Secret)
		return !reflect.DeepEqual(oldSecret.Data, newSecret.Data)
	},
}
var CMContentChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldCM := e.ObjectOld.(*corev1.ConfigMap)
		newCM := e.ObjectNew.(*corev1.ConfigMap)
		return !reflect.DeepEqual(oldCM.Data, newCM.Data)
	},
}

var DSCDeletionPredicate = predicate.Funcs{
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

func (r *DSCInitializationReconciler) watchMonitoringConfigMapResrouce(a client.Object) (requests []reconcile.Request) {
	if a.GetName() == "prometheus" && a.GetNamespace() == "redhat-ods-monitoring" {
		r.Log.Info("Found monitoring configmap has updated, start reconcile")
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: "prometheus", Namespace: "redhat-ods-monitoring"},
		}}
	} else {
		return nil
	}
}

func (r *DSCInitializationReconciler) watchMonitoringSecretResrouce(a client.Object) (requests []reconcile.Request) {
	operatorNs, err := upgrade.GetOperatorNamespace()
	if err != nil {
		return nil
	}
	if a.GetName() == "addon-managed-odh-parameters" && a.GetNamespace() == operatorNs {
		r.Log.Info("Found monitoring secret has updated, start reconcile")
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: "addon-managed-odh-parameters", Namespace: operatorNs},
		}}
	} else {
		return nil
	}
}

func (r *DSCInitializationReconciler) watchDSCResrouce(a client.Object) (requests []reconcile.Request) {
	instanceList := &dscv1.DataScienceClusterList{}
	if err := r.Client.List(context.TODO(), instanceList); err != nil {
		// do not handle if cannot get list
		return nil
	}
	if len(instanceList.Items) == 0 {
		r.Log.Info("Found no DSC instance in cluster, reset monitoring stack config")
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: "backup"},
		}}
	}
	return nil
}
