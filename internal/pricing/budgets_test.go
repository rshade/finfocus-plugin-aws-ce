package pricing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetBudgetsFakeEndpoint(t *testing.T) {
	t.Setenv(AccountIDEnv, "123456789012")
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "123456789012") {
			t.Errorf("account id missing: %s", body)
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		pages++
		if pages == 1 {
			_, _ = w.Write([]byte(`{"Budgets":[{"BudgetName":"monthly","BudgetLimit":{"Amount":"100","Unit":"USD"},"TimeUnit":"MONTHLY","CalculatedSpend":{"ActualSpend":{"Amount":"40","Unit":"USD"},"ForecastedSpend":{"Amount":"90","Unit":"USD"}}}],"NextToken":"page-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"Budgets":[{"BudgetName":"empty-status","BudgetLimit":{"Amount":"10","Unit":"USD"},"TimeUnit":"MONTHLY"}]}`))
	}))
	t.Cleanup(server.Close)

	budgetsClient, err := client.NewBudgetsClient(context.Background(), client.Config{
		Region:          "us-east-1",
		BaseEndpoint:    server.URL,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculator()
	calc.budgets = budgetsClient
	resp, err := calc.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{IncludeStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(resp.GetBudgets()) != 2 {
		t.Fatalf("pages=%d budgets=%d", pages, len(resp.GetBudgets()))
	}
	first := resp.GetBudgets()[0]
	if first.GetName() != "monthly" || first.GetAmount().GetLimit() != 100 || first.GetStatus().GetCurrentSpend() != 40 {
		t.Fatalf("first=%v", first)
	}
	if first.GetStatus().GetForecastedSpend() != 90 || first.GetPeriod() != pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY {
		t.Fatalf("status=%v period=%v", first.GetStatus(), first.GetPeriod())
	}
	if resp.GetSummary().GetTotalBudgets() != 2 {
		t.Fatalf("summary=%v", resp.GetSummary())
	}
}

func TestGetBudgetsEmpty(t *testing.T) {
	t.Setenv(AccountIDEnv, "123456789012")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"Budgets":[]}`))
	}))
	t.Cleanup(server.Close)
	budgetsClient, err := client.NewBudgetsClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculator()
	calc.budgets = budgetsClient
	resp, err := calc.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetBudgets()) != 0 || resp.GetSummary().GetTotalBudgets() != 0 {
		t.Fatalf("resp=%v", resp)
	}
}

func TestGetBudgetsPermissionDenied(t *testing.T) {
	t.Setenv(AccountIDEnv, "123456789012")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"AccessDeniedException","Message":"secret denied body"}`))
	}))
	t.Cleanup(server.Close)
	budgetsClient, err := client.NewBudgetsClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculator()
	calc.budgets = budgetsClient
	_, err = calc.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", err)
	}
	if strings.Contains(status.Convert(err).Message(), "secret") {
		t.Fatalf("status leaked the AWS message: %v", err)
	}
}
