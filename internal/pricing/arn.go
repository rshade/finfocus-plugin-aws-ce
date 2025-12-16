package pricing

import (
	"fmt"
	"strings"
)

// ParsedARN represents the components of an Amazon Resource Name.
type ParsedARN struct {
	Partition string
	Service   string
	Region    string
	AccountID string
	Resource  string
}

// ParseARN parses an ARN string into its components.
func ParseARN(arn string) (*ParsedARN, error) {
	if !strings.HasPrefix(arn, "arn:") {
		return nil, fmt.Errorf("invalid arn: does not start with 'arn:'")
	}
	
	// Split into at most 6 parts: arn:partition:service:region:account:resource
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return nil, fmt.Errorf("invalid arn: malformed format")
	}

	return &ParsedARN{
		Partition: parts[1],
		Service:   parts[2],
		Region:    parts[3],
		AccountID: parts[4],
		Resource:  parts[5],
	}, nil
}
