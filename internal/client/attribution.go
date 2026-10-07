package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

type costIdentity struct{ reservation, savings string }

// GetAttributedCost partitions costs by commitment filters, then queries the
// remaining costs. SERVICE is a supported grouping; commitment IDs are filters,
// not groupings. Each charge belongs to only one partition. Commitment partitions
// use AmortizedCost when returned; the residual uses UnblendedCost.
func (c *Client) GetAttributedCost(ctx context.Context, filter *types.Expression, start, end time.Time, granularity string) ([]CostResult, error) {
	return c.attributedCosts(ctx, filter, start, end, func(part *types.Expression, identity costIdentity) ([]CostResult, error) {
		return c.getCost(ctx, part, []string{dimService}, start, end, granularity, identity)
	})
}

// GetAttributedCostWithResources enriches EC2 resource costs using commitment
// filters. Discovery is scoped to EC2 and account; resource IDs remain confined
// to GetCostAndUsageWithResources, which supports resource-level filtering.
func (c *Client) GetAttributedCostWithResources(ctx context.Context, resourceID, accountID string, start, end time.Time) ([]CostResult, error) {
	filter := dimensionFilter(types.DimensionService, []string{ec2ComputeService})
	if accountID != "" {
		filter = andFilters(filter, dimensionFilter(types.DimensionLinkedAccount, []string{accountID}))
	}
	return c.attributedCosts(ctx, filter, start, end, func(part *types.Expression, identity costIdentity) ([]CostResult, error) {
		return c.getCostWithResources(ctx, resourceID, accountID, start, end, part, identity)
	})
}

func (c *Client) attributedCosts(ctx context.Context, filter *types.Expression, start, end time.Time, query func(*types.Expression, costIdentity) ([]CostResult, error)) ([]CostResult, error) {
	interval, err := dateInterval(start, end)
	if err != nil {
		return nil, err
	}
	reservations, err := c.commitmentIDs(ctx, filter, interval, types.DimensionReservationId)
	if err != nil {
		return nil, err
	}
	savings, err := c.commitmentIDs(ctx, filter, interval, types.DimensionSavingsPlanArn)
	if err != nil {
		return nil, err
	}
	var rows []CostResult
	reservationFilter := dimensionFilter(types.DimensionReservationId, reservations)
	savingsFilter := dimensionFilter(types.DimensionSavingsPlanArn, savings)
	for _, id := range reservations {
		part := andFilters(filter, dimensionFilter(types.DimensionReservationId, []string{id}))
		costs, err := query(part, costIdentity{reservation: id})
		if err != nil && !errors.Is(err, ErrNoCostData) {
			return nil, err
		}
		rows = append(rows, costs...)
	}
	for _, id := range savings {
		part := andFilters(filter, dimensionFilter(types.DimensionSavingsPlanArn, []string{id}), notFilter(reservationFilter))
		costs, err := query(part, costIdentity{savings: id})
		if err != nil && !errors.Is(err, ErrNoCostData) {
			return nil, err
		}
		rows = append(rows, costs...)
	}
	residual := andFilters(filter, notFilter(reservationFilter), notFilter(savingsFilter))
	costs, err := query(residual, costIdentity{})
	if err != nil && !errors.Is(err, ErrNoCostData) {
		return nil, err
	}
	rows = append(rows, costs...)
	if len(rows) == 0 {
		return nil, ErrNoCostData
	}
	return rows, nil
}

func (c *Client) commitmentIDs(ctx context.Context, filter *types.Expression, interval *types.DateInterval, dimension types.Dimension) ([]string, error) {
	var token *string
	var ids []string
	seen := make(map[string]bool)
	for page := 0; page < 100; page++ {
		input := &costexplorer.GetDimensionValuesInput{TimePeriod: interval, Dimension: dimension, Context: types.ContextCostAndUsage, Filter: filter, NextPageToken: token}
		out, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetDimensionValuesOutput, error, bool) {
			if err := runPageHook(ctx); err != nil {
				return nil, err, false
			}
			out, err := c.ceClient.GetDimensionValues(ctx, input)
			return out, err, isRetryableError(err)
		})
		if err != nil {
			return nil, fmt.Errorf("discovering %s: %w", dimension, err)
		}
		if out == nil {
			return nil, fmt.Errorf("discovering %s: empty response", dimension)
		}
		for _, item := range out.DimensionValues {
			id := strings.TrimSpace(aws.ToString(item.Value))
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		token = out.NextPageToken
		if token == nil || *token == "" {
			return ids, nil
		}
	}
	return nil, ErrPageCap
}

func dimensionFilter(dimension types.Dimension, values []string) *types.Expression {
	if len(values) == 0 {
		return nil
	}
	return &types.Expression{Dimensions: &types.DimensionValues{Key: dimension, Values: values}}
}

func notFilter(filter *types.Expression) *types.Expression {
	if filter == nil {
		return nil
	}
	return &types.Expression{Not: filter}
}

func andFilters(filters ...*types.Expression) *types.Expression {
	var parts []types.Expression
	for _, filter := range filters {
		if filter != nil {
			parts = append(parts, *filter)
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return &parts[0]
	default:
		return &types.Expression{And: parts}
	}
}
