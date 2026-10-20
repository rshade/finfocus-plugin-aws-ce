package pricing

import (
	"context"
	"testing"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPricingSpecUnsupported(t *testing.T) {
	calculator := NewCalculator()
	for _, request := range []*pbc.GetPricingSpecRequest{
		nil,
		{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: "i-0123456789abcdef0"}},
	} {
		response, err := calculator.GetPricingSpec(context.Background(), request)
		if status.Code(err) != codes.Unimplemented || response != nil {
			t.Fatalf("actual-only pricing spec: response=%v error=%v, want Unimplemented", response, err)
		}
	}
}
