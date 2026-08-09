package osiris

import (
	"context"
	"testing"
)

// BenchmarkRuntimeNativeLoop is the stable in-process profiling target for
// workloads. CLI/process startup is intentionally excluded; bench.sh measures
// those separately.
func BenchmarkRuntimeNativeLoop(b *testing.B) {
	prog, err := Parse("benchmarks/native-loop.osr", `
let total = 0
let i = 0
while i < 100000 {
  let total = total + i
  let i = i + 1
}
emit total
`)
	if err != nil {
		b.Fatal(err)
	}
	rt := NewRuntime()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rt.Run(context.Background(), prog); err != nil {
			b.Fatal(err)
		}
	}
}
