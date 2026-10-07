package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
)

func batchSettingsFromEnv() (int, int, error) {
	size, err := batchSetting("FINFOCUS_AWS_CE_MAX_BATCH_SIZE", pluginsdk.DefaultMaxBatchSize, pluginsdk.MaxBatchSize)
	if err != nil {
		return 0, 0, err
	}
	workers, err := batchSetting("FINFOCUS_AWS_CE_BATCH_WORKERS", pluginsdk.DefaultBatchWorkers, pluginsdk.MaxBatchWorkers)
	if err != nil {
		return 0, 0, err
	}
	return size, workers, nil
}

func batchSetting(name string, fallback, maximum int) (int, error) {
	value, _ := os.LookupEnv(name)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > maximum {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", name, maximum)
	}
	return number, nil
}
