package pricing

import "strings"

// costExplorerService maps an ARN service code to the Cost Explorer SERVICE
// value. Unknown codes are returned unchanged and marked unknown. Non-EC2
// RESOURCE_ID forms stay unverified even when the service name is known.
func costExplorerService(arnService string) (string, bool) {
	switch arnService {
	case "ec2":
		return "Amazon Elastic Compute Cloud - Compute", true
	case "s3":
		return "Amazon Simple Storage Service", true
	case "rds":
		return "Amazon Relational Database Service", true
	case "lambda":
		return "AWS Lambda", true
	default:
		return arnService, false
	}
}

// bareResourceID is the name after the first ":" or "/" in an ARN resource
// component. A resource with no separator is already bare. Alias and version
// suffixes stay on the bare id; that form is unverified.
func bareResourceID(resource string) string {
	resource = strings.TrimPrefix(resource, "/")
	if i := strings.IndexAny(resource, ":/"); i >= 0 {
		return resource[i+1:]
	}
	return resource
}

func uniqueIDs(ids ...string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
