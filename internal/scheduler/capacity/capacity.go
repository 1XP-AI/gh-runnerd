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

var errNotImplemented = errors.New("capacity arithmetic is not implemented")

// FixedTarget returns the configured fixed total.
func FixedTarget(count int64) (int64, error) {
	return 0, errNotImplemented
}

// AutoscaleTarget returns the assigned-job total clamped to the configured
// inclusive total bounds.
func AutoscaleTarget(assignedJobs, minTotal, maxTotal int64) (int64, error) {
	return 0, errNotImplemented
}

// AdditionalCapacity returns the largest requested worker count that fits all
// supplied budgets.
func AdditionalCapacity(requested int64, budgets []Budget) (int64, error) {
	return 0, errNotImplemented
}
