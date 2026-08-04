package osiris

import "testing"

func TestTask146NativeExactNumericParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task146-native-exact-numeric",
		Source: `emit dec("0.5")
emit bigint("92233720368547758081234567890")
emit primal(dual(2, 3) * 4)
emit tangent(dual(2, 3) * 4)
emit decode(encode(dec("0.5")))
emit decode(encode(bigint("92233720368547758081234567890")))
emit primal(decode(encode(dual(2, 3))))
emit tangent(decode(encode(dual(2, 3))))
`,
	})
}

func TestTask146NativeTensorCodecParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task146-native-tensor-codec",
		Source: `let real = tensor([2, 2], [1.0f64, 2.0f64, 3.0f64, 4.0f64])
let complex = tensor([2], [complex(1, 2), complex(3, 4)])
emit decode(encode(real))
emit decode(encode(complex))
emit shape(decode(encode(real)))
`,
	})
}

func TestTask146NativeReductionAndCumulativeParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task146-native-reductions",
		Source: `let values = [1, 2, 3, 4]
emit var(values)
emit median(values)
emit percentile(values, 75)
emit argmin(values)
emit argmax(values)
emit any([0, 0, 1])
emit all([1, 2, 3])
emit cumsum(values)
emit cumprod(values)
emit cumsum(tensor([2, 2], [1, 2, 3, 4]))
`,
	})
}
