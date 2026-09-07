// Package capacity contains pure arithmetic used by the scheduler.
package capacity

import "errors"

// Resources is a pair of non-negative resource quantities.
type Resources struct {
	CPUMillicores int64
	MemoryBytes   int64
}

// Budget describes the available resource headroom and the cost of one
// additional homogeneous worker.
type Budget struct {
	Limit     Resources
	Committed Resources
	PerWorker Resources
}

var (
	errNegative       = errors.New("capacity: values must be non-negative")
	errInvalidBounds  = errors.New("capacity: minimum total exceeds maximum total")
	errMissingBudgets = errors.New("capacity: at least one budget is required")
	errNoWorkerCost   = errors.New("capacity: at least one per-worker cost must be positive")
)

// FixedTarget returns the configured fixed total.
func FixedTarget(count int64) (int64, error) {
	if count < 0 {
		return 0, errNegative
	}
	return count, nil
}

// AutoscaleTarget returns the assigned-job total clamped to the configured
// inclusive total bounds.
func AutoscaleTarget(assignedJobs, minTotal, maxTotal int64) (int64, error) {
	if assignedJobs < 0 || minTotal < 0 || maxTotal < 0 {
		return 0, errNegative
	}
	if minTotal > maxTotal {
		return 0, errInvalidBounds
	}
	if assignedJobs < minTotal {
		return minTotal, nil
	}
	if assignedJobs > maxTotal {
		return maxTotal, nil
	}
	return assignedJobs, nil
}

// AdditionalCapacity returns the largest requested worker count that fits all
// supplied budgets.
func AdditionalCapacity(requested int64, budgets []Budget) (int64, error) {
	if len(budgets) == 0 {
		return 0, errMissingBudgets
	}

	var validationErr error
	if requested < 0 {
		validationErr = errNegative
	}
	hasWorkerCost := false
	overcommitted := false
	for _, budget := range budgets {
		if err := validateResources(budget.Limit); err != nil && validationErr == nil {
			validationErr = err
		}
		if err := validateResources(budget.Committed); err != nil && validationErr == nil {
			validationErr = err
		}
		if err := validateResources(budget.PerWorker); err != nil && validationErr == nil {
			validationErr = err
		}

		if budget.PerWorker.CPUMillicores > 0 || budget.PerWorker.MemoryBytes > 0 {
			hasWorkerCost = true
		}
		if budget.Committed.CPUMillicores > budget.Limit.CPUMillicores || budget.Committed.MemoryBytes > budget.Limit.MemoryBytes {
			overcommitted = true
		}
	}
	if validationErr != nil {
		return 0, validationErr
	}
	if !hasWorkerCost {
		return 0, errNoWorkerCost
	}
	if overcommitted {
		return 0, nil
	}

	capacity := requested
	for _, budget := range budgets {
		if budget.PerWorker.CPUMillicores > 0 {
			available := budget.Limit.CPUMillicores - budget.Committed.CPUMillicores
			bound := available / budget.PerWorker.CPUMillicores
			if bound < capacity {
				capacity = bound
			}
		}
		if budget.PerWorker.MemoryBytes > 0 {
			available := budget.Limit.MemoryBytes - budget.Committed.MemoryBytes
			bound := available / budget.PerWorker.MemoryBytes
			if bound < capacity {
				capacity = bound
			}
		}
	}
	return capacity, nil
}

func validateResources(resources Resources) error {
	if resources.CPUMillicores < 0 || resources.MemoryBytes < 0 {
		return errNegative
	}
	return nil
}
