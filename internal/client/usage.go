package client

import "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

// A single UsageType filter guarantees the query does not add unlike units.
// AND preserves that restriction. OR and NOT alone do not establish it.
func hasSingleUsageType(filter *types.Expression) bool {
	if filter == nil {
		return false
	}
	if filter.Dimensions != nil && filter.Dimensions.Key == types.DimensionUsageType && len(filter.Dimensions.Values) == 1 {
		return true
	}
	for i := range filter.And {
		if hasSingleUsageType(&filter.And[i]) {
			return true
		}
	}
	return false
}

func omitUsage(periods []types.ResultByTime) {
	for i := range periods {
		delete(periods[i].Total, metricUsageQuantity)
		for j := range periods[i].Groups {
			delete(periods[i].Groups[j].Metrics, metricUsageQuantity)
		}
	}
}
