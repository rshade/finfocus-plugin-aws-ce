package pricing

import (
	"reflect"
	"testing"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
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
	
	t.Logf("--- GetProjectedCostResponse ---")
	printFields(t, &pbc.GetProjectedCostResponse{})
}

func printFields(t *testing.T, v interface{}) {
	val := reflect.ValueOf(v).Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		t.Logf("  %s: %s", field.Name, field.Type)
	}
}