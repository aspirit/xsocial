package idgen

import "testing"

func newBenchGenerator(b *testing.B) Generator {
	b.Helper()
	gen, err := NewGenerator(&Config{MachineID: 1})
	if err != nil {
		b.Fatalf("NewGenerator: %v", err)
	}

	return gen
}

func BenchmarkNextID(b *testing.B) {
	gen := newBenchGenerator(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := gen.NextID(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNextShortID(b *testing.B) {
	gen := newBenchGenerator(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := gen.NextShortID(); err != nil {
			b.Fatal(err)
		}
	}
}
