package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
)

type recommendationMock struct {
	CostExplorerAPI
	call func(*string) (*string, error, bool)
}

func (m *recommendationMock) GetRightsizingRecommendation(_ context.Context, in *costexplorer.GetRightsizingRecommendationInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetRightsizingRecommendationOutput, error) {
	token, err, empty := m.call(in.NextPageToken)
	if empty {
		return nil, err
	}
	return &costexplorer.GetRightsizingRecommendationOutput{NextPageToken: token}, err
}
func (m *recommendationMock) GetSavingsPlansPurchaseRecommendation(_ context.Context, in *costexplorer.GetSavingsPlansPurchaseRecommendationInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error) {
	token, err, empty := m.call(in.NextPageToken)
	if empty {
		return nil, err
	}
	return &costexplorer.GetSavingsPlansPurchaseRecommendationOutput{NextPageToken: token}, err
}
func TestListRightsizingRecommendations(t *testing.T) {
	testRecommendationPages(t, func(ctx context.Context, c *Client) (int, error) {
		in := costexplorer.GetRightsizingRecommendationInput{Service: aws.String("AmazonEC2")}
		pages, err := c.ListRightsizingRecommendations(ctx, in)
		require.Nil(t, in.NextPageToken)
		return len(pages), err
	})
}
func TestListSavingsPlansRecommendations(t *testing.T) {
	testRecommendationPages(t, func(ctx context.Context, c *Client) (int, error) {
		in := costexplorer.GetSavingsPlansPurchaseRecommendationInput{}
		pages, err := c.ListSavingsPlansRecommendations(ctx, in)
		require.Nil(t, in.NextPageToken)
		return len(pages), err
	})
}
func testRecommendationPages(t *testing.T, list func(context.Context, *Client) (int, error)) {
	t.Helper()
	for _, tc := range []struct {
		name         string
		calls, pages int
		fail         bool
	}{
		{"two pages", 2, 2, false}, {"empty dataset", 1, 1, false}, {"nil output", 1, 0, true}, {"repeated token", 2, 0, true}, {"page cap", 100, 0, true}, {"canceled", 0, 0, true}, {"permission", 1, 0, true}, {"retry", 2, 1, false}, {"budget on retry", 1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, hooks := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.name == "canceled" {
				cancel()
			}
			ctx = WithPageHook(ctx, func() error {
				hooks++
				if tc.name == "budget on retry" && hooks == 2 {
					return ErrRateLimited
				}
				return nil
			})
			mock := &recommendationMock{call: func(token *string) (*string, error, bool) {
				calls++
				if calls == 1 {
					require.Nil(t, token)
				} else if tc.name == "two pages" {
					require.Equal(t, "next", aws.ToString(token))
				}
				switch tc.name {
				case "two pages":
					if calls == 1 {
						return aws.String("next"), nil, false
					}
				case "nil output":
					return nil, nil, true
				case "repeated token":
					return aws.String("same"), nil, false
				case "page cap":
					return aws.String(fmt.Sprint(calls)), nil, false
				case "permission":
					return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "secret-value"}, false
				case "retry", "budget on retry":
					if calls == 1 {
						return nil, &smithy.GenericAPIError{Code: "ThrottlingException"}, false
					}
				}
				return nil, nil, false
			}}
			pages, err := list(ctx, &Client{ceClient: mock})
			require.Equal(t, tc.calls, calls)
			require.Equal(t, tc.pages, pages)
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "secret-value")
				switch tc.name {
				case "canceled":
					require.ErrorIs(t, err, context.Canceled)
				case "budget on retry":
					require.ErrorIs(t, err, ErrRateLimited)
				case "page cap":
					require.ErrorIs(t, err, ErrPageCap)
				case "permission":
					var cause *smithy.GenericAPIError
					require.ErrorAs(t, err, &cause)
					require.Equal(t, "AccessDeniedException", cause.Code)
				}
			} else {
				require.NoError(t, err)
			}
			if tc.name == "retry" || tc.name == "budget on retry" {
				require.Equal(t, 2, hooks)
			}
		})
	}
}
