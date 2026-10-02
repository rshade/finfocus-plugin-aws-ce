package client

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

const ec2ComputeService = "Amazon Elastic Compute Cloud - Compute"

// costExplorerPeriod converts instants into Cost Explorer dates.
// Start is the UTC calendar day of start. End is exclusive: midnight UTC stays
// on that day, and any later time on a UTC day uses the next day.
func costExplorerPeriod(start, end time.Time) (string, string, error) {
	start = start.UTC()
	end = end.UTC()
	if !end.After(start) {
		return "", "", ErrInvalidTimeRange
	}
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	if !end.Equal(endDay) {
		endDay = endDay.AddDate(0, 0, 1)
	}
	return startDay.Format("2006-01-02"), endDay.Format("2006-01-02"), nil
}

func dateInterval(start, end time.Time) (*types.DateInterval, error) {
	startDate, endDate, err := costExplorerPeriod(start, end)
	if err != nil {
		return nil, err
	}
	return &types.DateInterval{
		Start: aws.String(startDate),
		End:   aws.String(endDate),
	}, nil
}
