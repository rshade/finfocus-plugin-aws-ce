package pricing

import (
	"reflect"
	"testing"

	pbc "github.com/rshade/pulumicost-spec/sdk/go/proto/pulumicost/v1"
)

func TestInspectProto(t *testing.T) {
	t.Logf("--- GetActualCostRequest ---")
	printFields(t, &pbc.GetActualCostRequest{})

	t.Logf("--- GetActualCostResponse ---")
	printFields(t, &pbc.GetActualCostResponse{})
	
	t.Logf("--- ActualCostResult ---")
	printFields(t, &pbc.ActualCostResult{})

	t.Logf("--- GetProjectedCostRequest ---")
	printFields(t, &pbc.GetProjectedCostRequest{})
}

func printFields(t *testing.T, v interface{}) {
	val := reflect.ValueOf(v).Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		t.Logf("  %s: %s", field.Name, field.Type)
	}
}