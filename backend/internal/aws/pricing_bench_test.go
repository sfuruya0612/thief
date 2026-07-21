package aws

import (
	"fmt"
	"testing"
)

// benchSavingsPlanRates generates n PriceRate values for dedupeSavingsPlanRates
// benchmarking. Consecutive pairs share every visible (JSON-serialized) field
// and differ only in the unexported Operation field, mirroring the real
// duplicate shape documented on dedupeSavingsPlanRates (issue 0052: RDS
// Oracle/Db2 multi-SKU rows that differ only by internal Operation code), so
// benchmarking with n rates measures dedup down to n/2 results.
func benchSavingsPlanRates(n int) []PriceRate {
	rates := make([]PriceRate, 0, n)
	for i := range n {
		base := i / 2
		rates = append(rates, PriceRate{
			RateID: fmt.Sprintf("offer-%d#usage-%d#Linux", base, base),
			Model:  "savings_plan",
			Group:  "Compute Savings Plans",
			Label:  fmt.Sprintf("m5.%dxlarge / Linux", base%8+1),
			Attributes: map[string]string{
				"instance_type":   fmt.Sprintf("m5.%dxlarge", base%8+1),
				"instance_family": "m5",
				"os":              "Linux",
			},
			Term: PriceTerm{
				Lease:   strPtr("1yr"),
				Payment: strPtr("No Upfront"),
			},
			Unit:       "Hrs",
			PriceUSD:   0.05 + float64(base%10)*0.001,
			UpfrontUSD: 0,
			Currency:   "USD",
			Operation:  fmt.Sprintf("CreateDBInstance:%04d", i), // duplicates differ only here
		})
	}
	return rates
}

// BenchmarkDedupeSavingsPlanRates measures dedupeSavingsPlanRates at a
// realistic per-region scale (n=200, per the "数百" estimate in issue 0058)
// and a 10x-larger scale (n=2000) to check the trend holds. issue 0058
// compared this key-building approach against a json.Marshal-based one at
// the same scales and measured roughly 2x lower ns/op and half the
// allocs/op, which is why dedupeSavingsPlanRates was written this way
// instead of marshaling each row to JSON.
func BenchmarkDedupeSavingsPlanRates(b *testing.B) {
	for _, n := range []int{200, 2000} {
		rates := benchSavingsPlanRates(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				dedupeSavingsPlanRates(rates)
			}
		})
	}
}
