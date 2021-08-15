package yaml

import (
	"testing"
)

type benchStruct struct {
	a int
	b string
	c map[string]float32
}

func BenchmarkMarshal(b *testing.B) {
	s := benchStruct{
		a: 5,
		b: "foobar",
		c: map[string]float32{
			"bish": 1.2,
			"bash": 3.4,
			"bosh": 5.6,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(s); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	yaml := []byte(`a: 5
b: "foobar"
c:
  bish: 1.2
  bash: 3.4
  bosh: 5.6
`)
	var s benchStruct

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Unmarshal(yaml, &s); err != nil {
			b.Fatal(err)
		}
	}
}
