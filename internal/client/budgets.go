package client

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgetsTypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// BudgetsAPI is the AWS Budgets surface this plugin calls.
type BudgetsAPI interface {
	DescribeBudgets(ctx context.Context, params *budgets.DescribeBudgetsInput, optFns ...func(*budgets.Options)) (*budgets.DescribeBudgetsOutput, error)
}

// BudgetsClient reads budgets. It does not create or change them.
type BudgetsClient struct {
	api BudgetsAPI
}

// NewBudgetsClient builds a Budgets client. BaseEndpoint selects a fake server in tests.
func NewBudgetsClient(ctx context.Context, cfg Config) (*BudgetsClient, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	if awsCfg.Region == "" {
		awsCfg.Region = "us-east-1"
	}
	api := budgets.NewFromConfig(awsCfg, func(options *budgets.Options) {
		options.Retryer = aws.NopRetryer{}
		if cfg.BaseEndpoint != "" {
			options.BaseEndpoint = aws.String(cfg.BaseEndpoint)
		}
	})
	return &BudgetsClient{api: api}, nil
}

// NewBudgetsClientWithAPI uses a supplied API, for tests.
func NewBudgetsClientWithAPI(api BudgetsAPI) *BudgetsClient {
	return &BudgetsClient{api: api}
}

// ListBudgets returns every budget page for the account.
func (b *BudgetsClient) ListBudgets(ctx context.Context, accountID string) ([]budgetsTypes.Budget, error) {
	var all []budgetsTypes.Budget
	var token *string
	for page := 0; page < 100; page++ {
		if err := runPageHook(ctx); err != nil {
			return nil, err
		}
		out, err := b.api.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
			AccountId: aws.String(accountID),
			NextToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("describing budgets: %w", err)
		}
		if out == nil {
			return all, nil
		}
		all = append(all, out.Budgets...)
		if out.NextToken == nil || *out.NextToken == "" {
			return all, nil
		}
		token = out.NextToken
	}
	return nil, fmt.Errorf("%w; budgets NextToken still set", ErrPageCap)
}

// CallerAccountID asks STS which account the credentials belong to.
func CallerAccountID(ctx context.Context, cfg Config) (string, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("loading AWS config: %w", err)
	}
	if awsCfg.Region == "" {
		awsCfg.Region = "us-east-1"
	}
	out, err := sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("reading caller account: %w", err)
	}
	account := aws.ToString(out.Account)
	if account == "" {
		return "", fmt.Errorf("reading caller account: empty account id")
	}
	return account, nil
}
