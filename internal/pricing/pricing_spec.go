package pricing

import (
	"context"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetPricingSpec rejects pricing specifications, which Cost Explorer does not supply.
func (c *Calculator) GetPricingSpec(ctx context.Context, _ *pbc.GetPricingSpecRequest) (*pbc.GetPricingSpecResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetPricingSpec")
	defer done()
	return nil, status.Error(codes.Unimplemented, "pricing specifications are unavailable; use the aws-public plugin")
}
