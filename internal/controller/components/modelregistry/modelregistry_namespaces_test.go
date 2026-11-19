//nolint:testpackage // Exercise the same actions used by the component reconciler.
package modelregistry

import (
	"os"
	"testing"

	componentApi "github.com/opendatahub-io/opendatahub-operator/v2/api/components/v1alpha1"
	dsciv1 "github.com/opendatahub-io/opendatahub-operator/v2/api/dscinitialization/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/matchers/jq"

	. "github.com/onsi/gomega"
)

func TestModelRegistryNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name         string
		registries   string
		applications string
	}{
		{name: "defaults", registries: "rhoai-model-registries", applications: "redhat-ods-applications"},
		{name: "existing namespace", registries: "model-registries", applications: "redhat-ods-applications"},
		{name: "custom namespaces", registries: "team-registries", applications: "team-applications"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			path := t.TempDir()
			g.Expect(os.CopyFS(path, os.DirFS("testdata/namespaces"))).To(Succeed())

			cl, err := fakeclient.New()
			g.Expect(err).NotTo(HaveOccurred())
			mr := &componentApi.ModelRegistry{}
			mr.Spec.RegistriesNamespace = tc.registries
			rr := types.ReconciliationRequest{
				Client:   cl,
				Instance: mr,
				DSCI: &dsciv1.DSCInitialization{Spec: dsciv1.DSCInitializationSpec{
					ApplicationsNamespace: tc.applications,
				}},
				Manifests: []types.ManifestInfo{{Path: path}},
			}

			g.Expect(customizeManifests(t.Context(), &rr)).To(Succeed())
			g.Expect(configureDependencies(t.Context(), &rr)).To(Succeed())
			render := newManifestRenderer()
			g.Expect(render(t.Context(), &rr)).To(Succeed())
			assertModelRegistryNamespaces(t, rr.Resources, tc.registries, tc.applications)

			// The cached result must retain the same separation on subsequent reconciles.
			rr.Resources = nil
			g.Expect(configureDependencies(t.Context(), &rr)).To(Succeed())
			g.Expect(render(t.Context(), &rr)).To(Succeed())
			assertModelRegistryNamespaces(t, rr.Resources, tc.registries, tc.applications)
		})
	}
}

func assertModelRegistryNamespaces(t *testing.T, rendered resources.UnstructuredList, registries, applications string) {
	t.Helper()
	g := NewWithT(t)
	const controller = "model-registry-operator-system"

	for _, namespace := range []string{controller, registries} {
		g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "Namespace" and .metadata.name == "%s"`, namespace)))
	}
	for _, kind := range []string{"Deployment", "ServiceAccount", "Service"} {
		g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "%s" and .metadata.namespace == "%s"`, kind, controller)))
	}
	for _, kind := range []string{"Role", "RoleBinding"} {
		g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "%s" and .metadata.namespace == "%s"`, kind, registries)))
	}
	for _, kind := range []string{"RoleBinding", "ClusterRoleBinding"} {
		g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "%s" and .subjects[0].namespace == "%s"`, kind, controller)))
	}
	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"} {
		g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "%s" and .webhooks[0].clientConfig.service.namespace == "%s"`, kind, controller)))
	}
	g.Expect(rendered).To(ContainElement(jq.Match(
		`.kind == "CustomResourceDefinition" and .spec.conversion.webhook.clientConfig.service.namespace == "%s"`, controller)))
	g.Expect(rendered).To(ContainElement(jq.Match(`.kind == "Template" and .metadata.namespace == "%s"`, applications)))
	g.Expect(rendered).To(ContainElement(jq.Match(
		`.kind == "Deployment" and (.spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "REGISTRIES_NAMESPACE") | .value == "%s")`,
		registries)))
	g.Expect(rendered).NotTo(ContainElement(jq.Match(
		`.kind == "Deployment" and .metadata.namespace == "%s"`, applications)))
}
