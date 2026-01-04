package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/frame/data"
)

// WaitForConditionWithResult polls a condition function until it returns a non-nil result or timeout occurs
// This is useful when you need to wait for a specific result from an operation.
func WaitForConditionWithResult[T any](
	ctx context.Context,
	condition func() (*T, error),
	timeout time.Duration,
	pollInterval time.Duration,
) (*T, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := condition()
		if err != nil {
			if !data.ErrorIsNoRows(err) {
				return result, err
			}
		} else {
			if result != nil {
				return result, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("condition not met within timeout of %v", timeout)
}
