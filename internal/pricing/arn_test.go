package pricing

import (
	"testing"
)

func TestParseARN_Valid(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		expected ParsedARN
	}{
		{
			name: "EC2 Instance",
			arn:  "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
			expected: ParsedARN{
				Partition: "aws",
				Service:   "ec2",
				Region:    "us-east-1",
				AccountID: "123456789012",
				Resource:  "instance/i-1234567890abcdef0",
			},
		},
		{
			name: "S3 Bucket",
			arn:  "arn:aws:s3:::my_bucket",
			expected: ParsedARN{
				Partition: "aws",
				Service:   "s3",
				Region:    "",
				AccountID: "",
				Resource:  "my_bucket",
			},
		},
		{
			name: "Lambda Function",
			arn:  "arn:aws:lambda:us-west-2:123456789012:function:my-function",
			expected: ParsedARN{
				Partition: "aws",
				Service:   "lambda",
				Region:    "us-west-2",
				AccountID: "123456789012",
				Resource:  "function:my-function",
			},
		},
		{
			name: "IAM Role",
			arn:  "arn:aws:iam::123456789012:role/my-role",
			expected: ParsedARN{
				Partition: "aws",
				Service:   "iam",
				Region:    "",
				AccountID: "123456789012",
				Resource:  "role/my-role",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseARN(tc.arn)
			if err != nil {
				t.Fatalf("ParseARN failed: %v", err)
			}
			if *parsed != tc.expected {
				t.Errorf("Expected %+v, got %+v", tc.expected, *parsed)
			}
		})
	}
}

func TestParseARN_Invalid(t *testing.T) {
	tests := []struct {
		name string
		arn  string
	}{
		{"Empty", ""},
		{"Not ARN", "i-12345"},
		{"Short ARN", "arn:aws:ec2"},
		{"Missing Components", "arn:aws:ec2:region:account"}, // Needs at least 6 parts
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseARN(tc.arn)
			if err == nil {
				t.Error("Expected error for invalid ARN, got nil")
			}
		})
	}
}
