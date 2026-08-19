//nolint:testpackage
package setup

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/upgrade"

	. "github.com/onsi/gomega"
)

func TestFilterDeleteConfigMapPredicate(t *testing.T) {
	const operatorNs = "test-operator-ns"

	r := &SetupControllerReconciler{}
	preds := r.filterDeleteConfigMap(operatorNs)

	tests := []struct {
		name string
		obj  *corev1.ConfigMap
		want bool
	}{
		{
			name: "ConfigMap with correct namespace and label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-cm",
					Namespace: operatorNs,
					Labels:    map[string]string{upgrade.DeleteConfigMapLabel: "true"},
				},
			},
			want: true,
		},
		{
			name: "ConfigMap wrong namespace",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-cm",
					Namespace: "other-ns",
					Labels:    map[string]string{upgrade.DeleteConfigMapLabel: "true"},
				},
			},
			want: false,
		},
		{
			name: "ConfigMap missing label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-cm",
					Namespace: operatorNs,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(preds.Create(event.CreateEvent{Object: tt.obj})).
				To(Equal(tt.want), "CreateFunc")

			g.Expect(preds.Update(event.UpdateEvent{ObjectNew: tt.obj})).
				To(Equal(tt.want), "UpdateFunc")
		})
	}
}
