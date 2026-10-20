package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	spotAdvisorURL = "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json"
	spotAdvisorTTL = time.Hour
)

type httpAdvisor struct {
	client  *http.Client
	url     string
	ttl     time.Duration
	mu      sync.Mutex
	fetched time.Time
	doc     advisorDocument
}

type advisorDocument struct {
	Ranges []struct {
		Index int    `json:"index"`
		Label string `json:"label"`
	} `json:"ranges"`
	SpotAdvisor map[string]map[string]map[string]struct {
		R int `json:"r"`
	} `json:"spot_advisor"`
}

var defaultSpotAdvisor = newHTTPAdvisor(&http.Client{Timeout: 10 * time.Second}, spotAdvisorURL, spotAdvisorTTL)

func newHTTPAdvisor(client *http.Client, url string, ttl time.Duration) *httpAdvisor {
	if client == nil {
		client = http.DefaultClient
	}
	return &httpAdvisor{client: client, url: url, ttl: ttl}
}

func (a *httpAdvisor) Interruption(ctx context.Context, region, operatingSystem, instanceType string) (spotInterruption, error) {
	doc, err := a.document(ctx)
	if err != nil {
		return spotInterruption{}, err
	}
	row, ok := doc.SpotAdvisor[region][operatingSystem][instanceType]
	if !ok {
		return spotInterruption{}, nil
	}
	label := interruptionLabel(doc, row.R)
	score, set := interruptionScore(row.R)
	return spotInterruption{Label: label, Score: score, Known: set || label != ""}, nil
}

func (a *httpAdvisor) document(ctx context.Context) (advisorDocument, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.fetched.IsZero() && time.Since(a.fetched) < a.ttl {
		return a.doc, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
	if err != nil {
		return advisorDocument{}, err
	}
	req.Header.Set("User-Agent", "finfocus-plugin-aws-ce")
	resp, err := a.client.Do(req)
	if err != nil {
		return advisorDocument{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return advisorDocument{}, fmt.Errorf("spot advisor feed status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return advisorDocument{}, err
	}
	var doc advisorDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return advisorDocument{}, err
	}
	a.doc = doc
	a.fetched = time.Now()
	return doc, nil
}

func interruptionScore(index int) (float64, bool) {
	switch index {
	case 0:
		return 0.05, true
	case 1:
		return 0.10, true
	case 2:
		return 0.15, true
	case 3:
		return 0.20, true
	default:
		return 0, false
	}
}

func interruptionLabel(doc advisorDocument, index int) string {
	for _, bucket := range doc.Ranges {
		if bucket.Index == index && bucket.Label != "" {
			return bucket.Label
		}
	}
	labels := []string{"<5%", "5-10%", "10-15%", "15-20%", ">20%"}
	if index >= 0 && index < len(labels) {
		return labels[index]
	}
	return ""
}
