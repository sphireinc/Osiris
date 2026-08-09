package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeCoreDirectLoweringInventory(t *testing.T) {
	for _, builtin := range []string{
		"uint", "byte", "bytes", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64",
		"f32", "f64", "complex", "dual", "primal", "tangent", "conjugate", "magnitude",
		"atan2", "exp2", "trunc", "sign", "step", "heaviside", "relu", "sigmoid", "softplus",
		"mean", "std", "var", "median", "quantile", "percentile", "argmin", "argmax", "any", "all",
		"cumsum", "cumprod", "phase", "zeros", "ones", "full", "identity", "eye", "arange",
		"linspace", "logspace", "at", "setat", "tensor", "matrix", "reshape", "transpose", "matmul",
		"outer", "trace", "det", "solve", "inv", "cholesky", "qr", "shape", "flatten", "sum",
		"product", "dot", "norm", "gcd", "lcm", "count_ones", "count_zeros", "leading_zeros",
		"trailing_zeros", "bit_width", "bit_floor", "bit_ceil", "has_single_bit", "byte_swap",
	} {
		if got := nativeBuiltinStatus(builtin); got != BackendImplemented {
			t.Errorf("builtin %s inventory status = %q, want implemented", builtin, got)
		}
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "builtin/"+builtin)
		if !ok {
			t.Errorf("builtin/%s is missing from the capability inventory", builtin)
		} else if entry.Native != BackendImplemented {
			t.Errorf("builtin/%s inventory entry = %+v, want implemented native status", builtin, entry)
		}
	}

	for _, member := range []string{
		"uint", "byte", "complex", "dual", "primal", "tangent", "real", "imag", "conjugate", "magnitude",
		"atan2", "exp2", "trunc", "sign", "step", "heaviside", "relu", "sigmoid", "softplus", "mean",
		"std", "var", "median", "quantile", "percentile", "argmin", "argmax", "any", "all", "cumsum",
		"cumprod", "phase", "zeros", "ones", "full", "identity", "eye", "arange", "linspace", "logspace",
		"tensor", "matrix", "reshape", "transpose", "matmul", "outer", "trace", "det", "solve", "inv",
		"cholesky", "qr", "shape", "flatten", "dot", "norm", "gcd", "lcm",
		"count_ones", "count_zeros", "leading_zeros", "trailing_zeros", "bit_width", "bit_floor", "bit_ceil",
		"has_single_bit", "byte_swap",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/math/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Errorf("std/math.%s inventory entry = %+v, want implemented native status", member, entry)
		}
	}
}

func TestNativeCoreDirectLoweringContract(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		markers []string
	}{
		{
			name: "numeric constructors and projections",
			source: `emit uint(3)
emit byte(7)
emit bytes(encode("abc"))
emit complex(1, 2)
emit conjugate(complex(2, 3))
emit magnitude(complex(3, 4))
emit dual(3, 1)
emit primal(dual(3, 1))
emit tangent(dual(3, 1))
`,
			markers: []string{
				"ouintcast(", "obytecast(", "obytes(", "ocomplexcall(", "oconjugate(",
				"omagnitude(", "odualcall(", "oprimal(", "otangent(",
			},
		},
		{
			name: "tensor constructors and operations",
			source: `let value = matrix(2, 2, [1, 2, 3, 4])
emit zeros([2, 2])
emit ones(2, 2)
emit full([2, 2], 3)
emit identity(2)
emit eye(2)
emit arange(3)
emit linspace(0.0, 1.0, 3)
emit logspace(0.0, 2.0, 3)
emit at(value, [0, 1])
emit setat(value, [0, 1], 9)
emit reshape(value, [4])
emit transpose(value)
emit matmul(value, value)
emit outer([1, 2], [3, 4])
emit trace(value)
emit det(value)
emit solve(value, [5, 11])
emit inv(value)
emit cholesky(value)
emit qr(value)
emit shape(value)
emit flatten(value)
emit sum(value)
emit product(value)
emit dot(value, value)
emit norm(value)
`,
			markers: []string{
				"omatrixcall(", "ozeros(", "oones(", "ofull(", "oidentity(", "oarange(",
				"olinspace(", "ologspace(", "oat(", "osetat(", "oreshape(", "otranspose(",
				"omatmul(", "oouter(", "otrace(", "odet(", "osolve(", "oinv(",
				"ocholesky(", "oqr(", "oshape(", "oflatten(", "otensorsum(",
				"otensorproduct(", "otensordot(", "otensornorm(",
			},
		},
		{
			name: "numeric reductions",
			source: `let values = [1, 2, 3, 4]
emit mean(values)
emit std(values)
emit var(values)
emit median(values)
emit quantile(values, 0.5)
emit percentile(values, 50)
emit argmin(values)
emit argmax(values)
emit any([0, 0, 1])
emit all([1, 2, 3])
emit cumsum(values)
emit cumprod(values)
emit phase(complex(0, 1))
`,
			markers: []string{
				"omean(", "ostd(", "ovar(", "omedian(", "oquantile(", "opercentile(",
				"oargmin(", "oargmax(", "oany(", "oall(", "ocumsum(", "ocumprod(", "ophase(",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := Parse("<native-direct-lowering-contract>", tc.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			mainSource := artifact.Source
			if start := strings.Index(mainSource, "int main(void)"); start >= 0 {
				mainSource = mainSource[start:]
			}
			for _, marker := range tc.markers {
				if !strings.Contains(mainSource, marker) {
					t.Errorf("generated main omitted direct helper %q", marker)
				}
			}
			if strings.Contains(mainSource, `ostd_dispatch("std/math"`) || strings.Contains(mainSource, "ocall(") {
				t.Fatal("pure numeric lowering crossed an indirect native boundary")
			}
			if strings.Contains(mainSource, "#define OSIRIS_CAP_") {
				t.Fatal("pure numeric lowering unexpectedly defined a host capability")
			}
		})
	}
}
