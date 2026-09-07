package capacity

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

func TestFixedTarget(t *testing.T) {
	tests := []struct {
		name    string
		count   int64
		want    int64
		wantErr bool
	}{
		{name: "zero", count: 0, want: 0},
		{name: "positive", count: 7, want: 7},
		{name: "maximum", count: 1<<63 - 1, want: 1<<63 - 1},
		{name: "negative", count: -1, wantErr: true},
		{name: "minimum", count: -1 << 63, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FixedTarget(test.count)
			if (err != nil) != test.wantErr {
				t.Fatalf("FixedTarget(%d) error = %v, wantErr %t", test.count, err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("FixedTarget(%d) = %d, want %d", test.count, got, test.want)
			}
		})
	}
}

func TestAutoscaleTarget(t *testing.T) {
	tests := []struct {
		name                             string
		assignedJobs, minTotal, maxTotal int64
		want                             int64
		wantErr                          bool
	}{
		{name: "all zero", assignedJobs: 0, minTotal: 0, maxTotal: 0, want: 0},
		{name: "assigned below minimum", assignedJobs: 2, minTotal: 5, maxTotal: 9, want: 5},
		{name: "assigned inside bounds", assignedJobs: 7, minTotal: 5, maxTotal: 9, want: 7},
		{name: "assigned above maximum", assignedJobs: 12, minTotal: 5, maxTotal: 9, want: 9},
		{name: "maximum boundary", assignedJobs: 1<<63 - 1, minTotal: 1<<63 - 1, maxTotal: 1<<63 - 1, want: 1<<63 - 1},
		{name: "negative assigned", assignedJobs: -1, minTotal: 0, maxTotal: 1, wantErr: true},
		{name: "negative minimum", assignedJobs: 0, minTotal: -1, maxTotal: 1, wantErr: true},
		{name: "negative maximum", assignedJobs: 0, minTotal: 0, maxTotal: -1, wantErr: true},
		{name: "minimum exceeds maximum", assignedJobs: 0, minTotal: 2, maxTotal: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AutoscaleTarget(test.assignedJobs, test.minTotal, test.maxTotal)
			if (err != nil) != test.wantErr {
				t.Fatalf("AutoscaleTarget(%d, %d, %d) error = %v, wantErr %t", test.assignedJobs, test.minTotal, test.maxTotal, err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("AutoscaleTarget(%d, %d, %d) = %d, want %d", test.assignedJobs, test.minTotal, test.maxTotal, got, test.want)
			}
		})
	}
}

func TestAdditionalCapacityExamples(t *testing.T) {
	maxInt := int64(1<<63 - 1)
	tests := []struct {
		name      string
		requested int64
		budgets   []Budget
		want      int64
	}{
		{
			name:      "zero request still accepts valid budget",
			requested: 0,
			budgets:   []Budget{runtimeBudget(100, 0, 10, 1_000)},
			want:      0,
		},
		{
			name:      "cpu is limiting",
			requested: 10,
			budgets:   []Budget{runtimeBudget(10, 2, 2, 1)},
			want:      4,
		},
		{
			name:      "memory is limiting",
			requested: 10,
			budgets: []Budget{{
				Limit:     Resources{CPUMillicores: 100, MemoryBytes: 100},
				Committed: Resources{MemoryBytes: 20},
				PerWorker: Resources{CPUMillicores: 1, MemoryBytes: 20},
			}},
			want: 4,
		},
		{
			name:      "requested caps available capacity",
			requested: 3,
			budgets:   []Budget{runtimeBudget(100, 0, 1, 1)},
			want:      3,
		},
		{
			name:      "exact last slot",
			requested: 10,
			budgets:   []Budget{runtimeBudget(7, 1, 3, 1)},
			want:      2,
		},
		{
			name:      "committed above limit returns zero",
			requested: 10,
			budgets:   []Budget{runtimeBudget(3, 4, 1, 1)},
			want:      0,
		},
		{
			name:      "zero cost dimension is unbounded",
			requested: 10,
			budgets: []Budget{{
				Limit:     Resources{CPUMillicores: 0, MemoryBytes: 100},
				Committed: Resources{CPUMillicores: 0, MemoryBytes: 1},
				PerWorker: Resources{CPUMillicores: 0, MemoryBytes: 10},
			}},
			want: 9,
		},
		{
			name:      "zero cost host plus positive runtime",
			requested: 10,
			budgets: []Budget{
				{Limit: Resources{CPUMillicores: 100}, Committed: Resources{CPUMillicores: 10}},
				runtimeBudget(100, 10, 30, 1),
			},
			want: 3,
		},
		{
			name:      "max int request and unit cost",
			requested: maxInt,
			budgets:   []Budget{runtimeBudget(maxInt, 0, 1, 0)},
			want:      maxInt,
		},
		{
			name:      "max int values divide without overflow",
			requested: maxInt,
			budgets: []Budget{{
				Limit:     Resources{CPUMillicores: maxInt, MemoryBytes: maxInt},
				Committed: Resources{},
				PerWorker: Resources{CPUMillicores: maxInt, MemoryBytes: maxInt},
			}},
			want: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AdditionalCapacity(test.requested, test.budgets)
			if err != nil {
				t.Fatalf("AdditionalCapacity(%d, ...) error = %v", test.requested, err)
			}
			if got != test.want {
				t.Fatalf("AdditionalCapacity(%d, ...) = %d, want %d", test.requested, got, test.want)
			}
		})
	}
}

func TestAdditionalCapacityValidation(t *testing.T) {
	positive := runtimeBudget(10, 0, 1, 1)
	negativeCases := []struct {
		name    string
		request int64
		budgets []Budget
	}{
		{name: "negative request", request: -1, budgets: []Budget{positive}},
		{name: "negative cpu limit", budgets: []Budget{withResource(positive, func(r *Resources) { r.CPUMillicores = -1 }, nil, nil)}},
		{name: "negative memory limit", budgets: []Budget{withResource(positive, func(r *Resources) { r.MemoryBytes = -1 }, nil, nil)}},
		{name: "negative committed cpu", budgets: []Budget{withResource(positive, nil, func(r *Resources) { r.CPUMillicores = -1 }, nil)}},
		{name: "negative committed memory", budgets: []Budget{withResource(positive, nil, func(r *Resources) { r.MemoryBytes = -1 }, nil)}},
		{name: "negative per worker cpu", budgets: []Budget{withResource(positive, nil, nil, func(r *Resources) { r.CPUMillicores = -1 })}},
		{name: "negative per worker memory", budgets: []Budget{withResource(positive, nil, nil, func(r *Resources) { r.MemoryBytes = -1 })}},
	}

	for _, test := range negativeCases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AdditionalCapacity(test.request, test.budgets); err == nil {
				t.Fatal("AdditionalCapacity accepted a negative input")
			}
		})
	}

	for _, test := range []struct {
		name    string
		request int64
		budgets []Budget
	}{
		{name: "missing budgets", request: 0},
		{
			name:    "all zero costs with zero request",
			request: 0,
			budgets: []Budget{{Limit: Resources{CPUMillicores: 1}, Committed: Resources{}}},
		},
		{
			name:    "all zero costs with positive request",
			request: 4,
			budgets: []Budget{{Limit: Resources{CPUMillicores: 1}, Committed: Resources{}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AdditionalCapacity(test.request, test.budgets); err == nil {
				t.Fatal("AdditionalCapacity accepted invalid budget input")
			}
		})
	}
}

func TestAdditionalCapacityValidatesLaterInputsAfterZeroBound(t *testing.T) {
	zeroBound := runtimeBudget(0, 0, 1, 1)
	invalidLater := runtimeBudget(10, 0, 1, 1)
	invalidLater.Limit.MemoryBytes = -1

	if _, err := AdditionalCapacity(10, []Budget{zeroBound, invalidLater}); err == nil {
		t.Fatal("AdditionalCapacity returned a zero result without validating the later budget")
	}
}

func TestAdditionalCapacityValidatesLaterInputsForZeroRequest(t *testing.T) {
	valid := runtimeBudget(10, 0, 1, 1)
	invalidLater := runtimeBudget(10, 0, 1, 1)
	invalidLater.Committed.CPUMillicores = -1

	if _, err := AdditionalCapacity(0, []Budget{valid, invalidLater}); err == nil {
		t.Fatal("AdditionalCapacity returned zero without validating the later budget")
	}
}

func TestAdditionalCapacityValidatesLaterInputsAfterOvercommit(t *testing.T) {
	overcommitted := runtimeBudget(0, 1, 1, 1)
	invalidLater := runtimeBudget(10, 0, 1, 1)
	invalidLater.PerWorker.MemoryBytes = -1

	if _, err := AdditionalCapacity(10, []Budget{overcommitted, invalidLater}); err == nil {
		t.Fatal("AdditionalCapacity returned a zero result without validating the later budget")
	}
}

func TestAdditionalCapacityDoesNotMutateInputs(t *testing.T) {
	budgets := []Budget{
		runtimeBudget(100, 10, 5, 20),
		{Limit: Resources{CPUMillicores: 90}, Committed: Resources{CPUMillicores: 9}},
	}
	original := append([]Budget(nil), budgets...)

	if _, err := AdditionalCapacity(10, budgets); err != nil {
		t.Fatalf("AdditionalCapacity(...) error = %v", err)
	}
	if !reflect.DeepEqual(budgets, original) {
		t.Fatalf("AdditionalCapacity mutated budgets: got %#v, want %#v", budgets, original)
	}
}

func TestAdditionalCapacityUsesIndependentBigOracle(t *testing.T) {
	tests := []struct {
		name      string
		requested int64
		budgets   []Budget
	}{
		{
			name:      "multiple resource bounds",
			requested: 100,
			budgets: []Budget{
				runtimeBudget(97, 13, 7, 11),
				{Limit: Resources{MemoryBytes: 101}, Committed: Resources{MemoryBytes: 2}, PerWorker: Resources{MemoryBytes: 9}},
			},
		},
		{
			name:      "headroom at maximum integer",
			requested: 1<<63 - 1,
			budgets: []Budget{{
				Limit:     Resources{CPUMillicores: 1<<63 - 1, MemoryBytes: 1<<63 - 1},
				PerWorker: Resources{CPUMillicores: 2, MemoryBytes: 3},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertCapacityMatchesBigOracle(t, test.requested, test.budgets)
		})
	}
}

func TestAdditionalCapacityProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(0x12a5eed))
	for iteration := 0; iteration < 512; iteration++ {
		requested := int64(rng.Intn(1_000))
		budgets := make([]Budget, 1+rng.Intn(4))
		for i := range budgets {
			cpuLimit := int64(rng.Intn(1_000))
			memoryLimit := int64(rng.Intn(1_000))
			budget := Budget{
				Limit: Resources{CPUMillicores: cpuLimit, MemoryBytes: memoryLimit},
				Committed: Resources{
					CPUMillicores: int64(rng.Int63n(cpuLimit + 1)),
					MemoryBytes:   int64(rng.Int63n(memoryLimit + 1)),
				},
				PerWorker: Resources{
					CPUMillicores: int64(rng.Intn(32)),
					MemoryBytes:   int64(rng.Intn(32)),
				},
			}
			if i == 0 && budget.PerWorker.CPUMillicores == 0 && budget.PerWorker.MemoryBytes == 0 {
				budget.PerWorker.CPUMillicores = 1
			}
			budgets[i] = budget
		}

		assertCapacityMatchesBigOracle(t, requested, budgets)
		got, err := AdditionalCapacity(requested, budgets)
		if err != nil {
			t.Fatalf("iteration %d: AdditionalCapacity(...) error = %v", iteration, err)
		}
		again, err := AdditionalCapacity(requested, budgets)
		if err != nil || again != got {
			t.Fatalf("iteration %d: repeated result = %d, %v; first = %d", iteration, again, err, got)
		}

		largerRequest := requested + 1
		larger, err := AdditionalCapacity(largerRequest, budgets)
		if err != nil || larger < got {
			t.Fatalf("iteration %d: request monotonicity got %d for %d after %d for %d (err %v)", iteration, larger, largerRequest, got, requested, err)
		}

		moreCPU := append([]Budget(nil), budgets...)
		moreCPU[0].Limit.CPUMillicores++
		moreHeadroom, err := AdditionalCapacity(requested, moreCPU)
		if err != nil || moreHeadroom < got {
			t.Fatalf("iteration %d: limit monotonicity got %d after %d (err %v)", iteration, moreHeadroom, got, err)
		}
	}
}

func FuzzAdditionalCapacityProperties(f *testing.F) {
	f.Add(int64(0), int64(10), int64(0), int64(1), int64(100), int64(0), int64(10))
	f.Add(int64(63), int64(127), int64(63), int64(7), int64(255), int64(1), int64(3))
	f.Add(int64(1<<62), int64(1<<62), int64(1<<61), int64(1), int64(1<<62), int64(0), int64(0))

	f.Fuzz(func(t *testing.T, requested, cpuLimit, cpuCommitted, cpuCost, memoryLimit, memoryCommitted, memoryCost int64) {
		req := int64(uint64(requested) % 2_000)
		limitCPU := int64(uint64(cpuLimit) % 2_000)
		limitMemory := int64(uint64(memoryLimit) % 2_000)
		committedCPU := int64(uint64(cpuCommitted) % uint64(limitCPU+1))
		committedMemory := int64(uint64(memoryCommitted) % uint64(limitMemory+1))
		costCPU := int64(uint64(cpuCost) % 32)
		costMemory := int64(uint64(memoryCost) % 32)
		if costCPU == 0 && costMemory == 0 {
			costCPU = 1
		}
		budgets := []Budget{{
			Limit:     Resources{CPUMillicores: limitCPU, MemoryBytes: limitMemory},
			Committed: Resources{CPUMillicores: committedCPU, MemoryBytes: committedMemory},
			PerWorker: Resources{CPUMillicores: costCPU, MemoryBytes: costMemory},
		}}

		assertCapacityMatchesBigOracle(t, req, budgets)
		before := append([]Budget(nil), budgets...)
		got, err := AdditionalCapacity(req, budgets)
		if err != nil {
			t.Fatalf("AdditionalCapacity(...) error = %v", err)
		}
		again, err := AdditionalCapacity(req, budgets)
		if err != nil || again != got {
			t.Fatalf("repeated call = %d, %v; first = %d", again, err, got)
		}
		if !reflect.DeepEqual(budgets, before) {
			t.Fatalf("AdditionalCapacity mutated budgets: got %#v, want %#v", budgets, before)
		}
		if req < 2_000 {
			larger, err := AdditionalCapacity(req+1, budgets)
			if err != nil || larger < got {
				t.Fatalf("request monotonicity got %d after %d (err %v)", larger, got, err)
			}
		}
	})
}

func assertCapacityMatchesBigOracle(t *testing.T, requested int64, budgets []Budget) {
	t.Helper()
	got, err := AdditionalCapacity(requested, budgets)
	if err != nil {
		t.Fatalf("AdditionalCapacity(...) error = %v", err)
	}
	want := bigCapacityOracle(requested, budgets)
	if got != want {
		t.Fatalf("AdditionalCapacity(...) = %d, big oracle = %d", got, want)
	}

	candidate := big.NewInt(got)
	requestedBig := big.NewInt(requested)
	if candidate.Sign() < 0 || candidate.Cmp(requestedBig) > 0 {
		t.Fatalf("candidate %d is outside [0, %d]", got, requested)
	}
	forEachResource(budgets, func(index int, dimension string, limit, committed, perWorker int64) {
		if perWorker == 0 {
			return
		}
		used := new(big.Int).Add(big.NewInt(committed), new(big.Int).Mul(new(big.Int).Set(candidate), big.NewInt(perWorker)))
		if used.Cmp(big.NewInt(limit)) > 0 {
			t.Errorf("budget %d %s is infeasible at %d workers: used %s > limit %d", index, dimension, got, used, limit)
		}
	})
	if got < requested {
		next := big.NewInt(got + 1)
		feasible := true
		forEachResource(budgets, func(_ int, _ string, limit, committed, perWorker int64) {
			if perWorker == 0 {
				return
			}
			used := new(big.Int).Add(big.NewInt(committed), new(big.Int).Mul(new(big.Int).Set(next), big.NewInt(perWorker)))
			if used.Cmp(big.NewInt(limit)) > 0 {
				feasible = false
			}
		})
		if feasible {
			t.Fatalf("candidate %d is not maximal below request %d", got, requested)
		}
	}
}

func bigCapacityOracle(requested int64, budgets []Budget) int64 {
	best := big.NewInt(requested)
	forEachResource(budgets, func(_ int, _ string, limit, committed, perWorker int64) {
		if perWorker == 0 {
			return
		}
		available := new(big.Int).Sub(big.NewInt(limit), big.NewInt(committed))
		bound := new(big.Int).Quo(available, big.NewInt(perWorker))
		if bound.Cmp(best) < 0 {
			best.Set(bound)
		}
	})
	return best.Int64()
}

func forEachResource(budgets []Budget, visit func(index int, dimension string, limit, committed, perWorker int64)) {
	for index, budget := range budgets {
		visit(index, "cpu", budget.Limit.CPUMillicores, budget.Committed.CPUMillicores, budget.PerWorker.CPUMillicores)
		visit(index, "memory", budget.Limit.MemoryBytes, budget.Committed.MemoryBytes, budget.PerWorker.MemoryBytes)
	}
}

func runtimeBudget(cpuLimit, cpuCommitted, cpuCost, memoryCost int64) Budget {
	return Budget{
		Limit:     Resources{CPUMillicores: cpuLimit, MemoryBytes: 1_000},
		Committed: Resources{CPUMillicores: cpuCommitted, MemoryBytes: 0},
		PerWorker: Resources{CPUMillicores: cpuCost, MemoryBytes: memoryCost},
	}
}

func withResource(b Budget, limit, committed, perWorker func(*Resources)) Budget {
	if limit != nil {
		limit(&b.Limit)
	}
	if committed != nil {
		committed(&b.Committed)
	}
	if perWorker != nil {
		perWorker(&b.PerWorker)
	}
	return b
}
