package dscinitialization_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/opendatahub-io/opendatahub-operator/v2/internal/controller/dscinitialization"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"

	. "github.com/onsi/gomega"
)

func TestManagedMonitoringByObjectScopesSingleSecretAndConfigMap(t *testing.T) {
	const operatorNs = "redhat-ods-operator"

	byObject := dscinitialization.ManagedMonitoringByObject(operatorNs)

	g := NewWithT(t)

	var secretRule, cmRule cache.ByObject
	foundSecret, foundCM := false, false

	for obj, rule := range byObject {
		switch obj.(type) {
		case *corev1.Secret:
			secretRule = rule
			foundSecret = true
		case *corev1.ConfigMap:
			cmRule = rule
			foundCM = true
		default:
			t.Fatalf("unexpected cache key type %T", obj)
		}
	}

	g.Expect(foundSecret).To(BeTrue())
	g.Expect(secretRule.Namespaces).To(HaveKey(operatorNs))
	g.Expect(secretRule.Namespaces[operatorNs].FieldSelector.String()).
		To(Equal(fields.OneTermEqualSelector("metadata.name", "addon-managed-odh-parameters").String()))

	g.Expect(foundCM).To(BeTrue())
	g.Expect(cmRule.Namespaces).To(HaveKey(cluster.DefaultMonitoringNamespaceRHOAI))
	g.Expect(cmRule.Namespaces[cluster.DefaultMonitoringNamespaceRHOAI].FieldSelector.String()).
		To(Equal(fields.OneTermEqualSelector("metadata.name", "prometheus").String()))
}
