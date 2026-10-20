package pricing

import (
	"context"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	budgetsTypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AccountIDEnv is the AWS account whose budgets are read. Empty uses STS GetCallerIdentity.
const AccountIDEnv = "FINFOCUS_AWS_ACCOUNT_ID"

type budgetLister interface {
	ListBudgets(ctx context.Context, accountID string) ([]budgetsTypes.Budget, error)
}

// GetBudgets reads AWS Budgets. It does not send alerts.
func (c *Calculator) GetBudgets(ctx context.Context, req *pbc.GetBudgetsRequest) (*pbc.GetBudgetsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetBudgets")
	defer done()

	if req != nil && req.GetFilter() != nil && !providerAllowed(req.GetFilter().GetProviders()) {
		return &pbc.GetBudgetsResponse{Budgets: []*pbc.Budget{}, Summary: &pbc.BudgetSummary{}}, nil
	}

	accountID := os.Getenv(AccountIDEnv)
	if accountID == "" {
		var err error
		accountID, err = c.lookupAccount(ctx)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, "budget account id is unavailable; set "+AccountIDEnv)
		}
	}

	lister := c.budgets
	if lister == nil {
		budgetsClient, err := client.NewBudgetsClient(ctx, client.Config{Region: "us-east-1"})
		if err != nil {
			return nil, status.Error(codes.Internal, "budget client initialization failed")
		}
		lister = budgetsClient
	}

	var pages int
	ctx = c.withCostBudget(ctx, &pages)
	rows, err := lister.ListBudgets(ctx, accountID)
	if err != nil {
		if mapped, ok := mapAWSAPIError(err); ok {
			return nil, mapped
		}
		return nil, status.Error(codes.Internal, "describing budgets failed")
	}

	out := make([]*pbc.Budget, 0, len(rows))
	summary := &pbc.BudgetSummary{}
	include := req.GetIncludeStatus()
	for _, row := range rows {
		mapped := mapBudget(row, include)
		out = append(out, mapped)
		summary.TotalBudgets++
		if mapped.GetStatus() != nil {
			switch mapped.GetStatus().GetHealth() {
			case pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_OK:
				summary.BudgetsOk++
			case pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_WARNING:
				summary.BudgetsWarning++
			case pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_CRITICAL:
				summary.BudgetsCritical++
			case pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_EXCEEDED:
				summary.BudgetsExceeded++
			}
		}
	}
	return &pbc.GetBudgetsResponse{Budgets: out, Summary: summary}, nil
}

func (c *Calculator) lookupAccount(ctx context.Context) (string, error) {
	if c.accountID != nil {
		return c.accountID(ctx)
	}
	return client.CallerAccountID(ctx, client.Config{Region: "us-east-1"})
}

func providerAllowed(providers []string) bool {
	if len(providers) == 0 {
		return true
	}
	for _, provider := range providers {
		if provider == "aws" {
			return true
		}
	}
	return false
}

func mapBudget(row budgetsTypes.Budget, includeStatus bool) *pbc.Budget {
	name := aws.ToString(row.BudgetName)
	amount := 0.0
	currency := "USD"
	if row.BudgetLimit != nil {
		amount, _ = strconv.ParseFloat(aws.ToString(row.BudgetLimit.Amount), 64)
		if unit := aws.ToString(row.BudgetLimit.Unit); unit != "" {
			currency = unit
		}
	}
	budget := &pbc.Budget{
		Id:     name,
		Name:   name,
		Source: "aws-budgets",
		Amount: &pbc.BudgetAmount{Limit: amount, Currency: currency},
		Period: budgetPeriod(row.TimeUnit),
	}
	if includeStatus && row.CalculatedSpend != nil {
		budget.Status = budgetStatus(amount, currency, row.CalculatedSpend)
	}
	return budget
}

func budgetPeriod(unit budgetsTypes.TimeUnit) pbc.BudgetPeriod {
	switch unit {
	case budgetsTypes.TimeUnitDaily:
		return pbc.BudgetPeriod_BUDGET_PERIOD_DAILY
	case budgetsTypes.TimeUnitMonthly:
		return pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY
	case budgetsTypes.TimeUnitQuarterly:
		return pbc.BudgetPeriod_BUDGET_PERIOD_QUARTERLY
	case budgetsTypes.TimeUnitAnnually:
		return pbc.BudgetPeriod_BUDGET_PERIOD_ANNUALLY
	default:
		return pbc.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED
	}
}

func budgetStatus(limit float64, currency string, spend *budgetsTypes.CalculatedSpend) *pbc.BudgetStatus {
	status := &pbc.BudgetStatus{Currency: currency}
	if spend.ActualSpend != nil {
		status.CurrentSpend, _ = strconv.ParseFloat(aws.ToString(spend.ActualSpend.Amount), 64)
		if unit := aws.ToString(spend.ActualSpend.Unit); unit != "" {
			status.Currency = unit
		}
	}
	if spend.ForecastedSpend != nil {
		status.ForecastedSpend, _ = strconv.ParseFloat(aws.ToString(spend.ForecastedSpend.Amount), 64)
	}
	if limit > 0 {
		status.PercentageUsed = status.CurrentSpend / limit * 100
		status.PercentageForecasted = status.ForecastedSpend / limit * 100
	}
	switch {
	case limit > 0 && status.CurrentSpend > limit:
		status.Health = pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_EXCEEDED
	case status.PercentageUsed >= 90:
		status.Health = pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_CRITICAL
	case status.PercentageUsed >= 75:
		status.Health = pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_WARNING
	default:
		status.Health = pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_OK
	}
	return status
}
