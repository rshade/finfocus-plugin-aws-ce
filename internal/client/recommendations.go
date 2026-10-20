package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
)

// RecommendationsAPI is the optional recommendation capability of a Cost Explorer client.
// Keeping it separate preserves compatibility with clients that only retrieve costs.
type RecommendationsAPI interface {
	GetRightsizingRecommendation(context.Context, *costexplorer.GetRightsizingRecommendationInput, ...func(*costexplorer.Options)) (*costexplorer.GetRightsizingRecommendationOutput, error)
	GetSavingsPlansPurchaseRecommendation(context.Context, *costexplorer.GetSavingsPlansPurchaseRecommendationInput, ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error)
}

// recommendationError retains the cause for classification without exposing AWS messages.
type recommendationError struct {
	operation string
	cause     error
}

func (e *recommendationError) Error() string {
	return e.operation + ": recommendation retrieval failed"
}
func (e *recommendationError) Unwrap() error { return e.cause }

// ListRightsizingRecommendations retrieves every page of EC2 rightsizing recommendations.
func (c *Client) ListRightsizingRecommendations(ctx context.Context, input costexplorer.GetRightsizingRecommendationInput) ([]costexplorer.GetRightsizingRecommendationOutput, error) {
	api, ok := c.ceClient.(RecommendationsAPI)
	if !ok {
		return nil, errors.New("cost explorer client does not support recommendations")
	}
	return recommendationPages(ctx, "getting rightsizing recommendations", input.NextPageToken, func(ctx context.Context, token *string) (*costexplorer.GetRightsizingRecommendationOutput, error) {
		pageInput := input
		pageInput.NextPageToken = token
		return api.GetRightsizingRecommendation(ctx, &pageInput)
	}, func(out *costexplorer.GetRightsizingRecommendationOutput) *string { return out.NextPageToken })
}

// ListSavingsPlansRecommendations retrieves every page of Savings Plans recommendations.
func (c *Client) ListSavingsPlansRecommendations(ctx context.Context, input costexplorer.GetSavingsPlansPurchaseRecommendationInput) ([]costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error) {
	api, ok := c.ceClient.(RecommendationsAPI)
	if !ok {
		return nil, errors.New("cost explorer client does not support recommendations")
	}
	return recommendationPages(ctx, "getting savings plans recommendations", input.NextPageToken, func(ctx context.Context, token *string) (*costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error) {
		pageInput := input
		pageInput.NextPageToken = token
		return api.GetSavingsPlansPurchaseRecommendation(ctx, &pageInput)
	}, func(out *costexplorer.GetSavingsPlansPurchaseRecommendationOutput) *string { return out.NextPageToken })
}

func recommendationPages[T any](ctx context.Context, operation string, token *string, fetch func(context.Context, *string) (*T, error), nextToken func(*T) *string) ([]T, error) {
	var pages []T
	seen := make(map[string]bool)
	if initial := aws.ToString(token); initial != "" {
		seen[initial] = true
	}
	for page := 0; page < 100; page++ {
		out, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*T, error, bool) {
			if err := ctx.Err(); err != nil {
				return nil, err, false
			}
			if err := runPageHook(ctx); err != nil {
				return nil, err, false
			}
			result, err := fetch(ctx, token)
			return result, err, isRetryableError(err)
		})
		if err != nil {
			return nil, &recommendationError{operation: operation, cause: err}
		}
		if out == nil {
			return nil, fmt.Errorf("%s: empty response", operation)
		}
		pages = append(pages, *out)
		next := aws.ToString(nextToken(out))
		if next == "" {
			return pages, nil
		}
		if seen[next] {
			return nil, fmt.Errorf("%s: repeated pagination token", operation)
		}
		seen[next] = true
		token = aws.String(next)
	}
	return nil, fmt.Errorf("%s: %w", operation, ErrPageCap)
}
