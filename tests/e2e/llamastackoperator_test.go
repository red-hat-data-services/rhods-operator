package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	componentApi "github.com/opendatahub-io/opendatahub-operator/v2/api/components/v1alpha1"
)

type LlamaStackOperatorTestCtx struct {
	*ComponentTestCtx
}

func llamastackOperatorTestSuite(t *testing.T) {
	t.Helper()

	ct, err := NewComponentTestCtx(t, &componentApi.LlamaStackOperator{})
	require.NoError(t, err)

	componentCtx := LlamaStackOperatorTestCtx{
		ComponentTestCtx: ct,
	}

	// Define test cases.
	testCases := []TestCase{
		{"Validate component enabled", componentCtx.ValidateComponentEnabled},
		{"Validate operands have OwnerReferences", componentCtx.ValidateOperandsOwnerReferences},
		{"Validate update operand resources", componentCtx.ValidateUpdateDeploymentsResources},
		{"Validate component releases", componentCtx.ValidateComponentReleases},
		{"Validate component disabled", componentCtx.ValidateComponentDisabled},
	}

	// Run the test suite.
	RunTestCases(t, testCases)
}
