package main

import (
	"testing"

	klabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	modelregistryctrl "github.com/opendatahub-io/opendatahub-operator/v2/internal/controller/components/modelregistry"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"

	. "github.com/onsi/gomega"
)

func TestModelRegistryNamespaceCache(t *testing.T) {
	g := NewWithT(t)
	existing := map[string]cache.Config{"redhat-ods-applications": {}}
	configured := withModelRegistryNamespaces(existing)

	// Existing namespaces keep their full watches; only the fallback is filtered.
	g.Expect(configured["redhat-ods-applications"]).To(Equal(existing["redhat-ods-applications"]))
	g.Expect(existing).NotTo(HaveKey(cache.AllNamespaces))
	g.Expect(configured).To(HaveKey(cache.AllNamespaces))
	selector := configured[cache.AllNamespaces].LabelSelector
	g.Expect(selector).NotTo(BeNil())
	g.Expect(selector.Matches(klabels.Set{
		labels.ODH.Component(modelregistryctrl.LegacyComponentName): labels.True,
	})).To(BeTrue())
	g.Expect(selector.Matches(klabels.Set{})).To(BeFalse())
	g.Expect(selector.Matches(klabels.Set{
		labels.ODH.Component("dashboard"): labels.True,
	})).To(BeFalse())
}
