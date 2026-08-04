package osiris

import "fmt"

// GPUCapabilityProfile describes the stable, device-independent GPU language
// surface. It is intentionally narrower than the hosted Value model: a GPU
// provider must reject values it cannot represent instead of silently changing
// width, precision, ownership, or host capabilities.
type GPUCapabilityProfile struct {
	Name        string
	ValueABI    string
	Stable      []string
	Unsupported map[string]string
}

// DefaultGPUCapabilityProfile is the portable profile shared by hardware and
// CPU-reference providers. Hardware providers may be stricter through device
// negotiation, but may not broaden this ABI without a new ABI version.
func DefaultGPUCapabilityProfile() GPUCapabilityProfile {
	return GPUCapabilityProfile{
		Name:     "deterministic-f64-tensor-v1",
		ValueABI: "scalar or row-major tensor of finite f64 values",
		Stable: []string{
			"core/emit", "core/let", "core/var", "core/functions", "core/ordered-control-flow",
			"numeric/f64", "array/vector", "array/tensor", "array/constructors", "array/ranges",
			"shape/reshape-transpose-flatten", "linear-algebra/dot-matmul", "reductions/statistics",
			"math/deterministic-real", "signal/hann-convolve", "stdlib/std-math-pure", "stdlib/std-signal-pure",
			"errors/capability-resource-cancellation", "execution/deterministic-ordering",
		},
		Unsupported: map[string]string{
			"numeric/complex":          "the v1 GPU value ABI has no complex lane type",
			"numeric/fixed-width":      "integer width and style would be lost by f64 transport",
			"numeric/decimal-big":      "exact decimal and arbitrary precision are not representable",
			"language/results-options": "structured result values are not GPU buffers",
			"language/modules":         "module loading is host-only; mapped stdlib calls are selected at compile time",
			"language/patterns":        "pattern matching is not a kernel operation",
			"language/tasks":           "tasks and streams require host scheduling and cancellation state",
			"stdlib/host-io":           "filesystem, networking, process, OS, time, text, and crypto are host-only",
			"stdlib/unsupported-math":  "complex, exact-integer, future, sparse, and provider-dependent math is rejected",
		},
	}
}

// ValidateGPUProgram checks only the source forms that can be represented by
// the GPU artifact contract. It does not execute the program or use an
// interpreter fallback; CompileGPU still validates the shared IR separately.
func ValidateGPUProgram(prog *Program) error {
	if prog == nil {
		return fmt.Errorf("GPU validate: nil program")
	}
	var unsupported string
	var walkExpr func(Expr)
	walkExpr = func(expr Expr) {
		if unsupported != "" || expr == nil {
			return
		}
		switch n := expr.(type) {
		case *callExpr:
			name := callName(n.callee)
			if gpuUnsupportedCall(name) {
				unsupported = name
				return
			}
			walkExpr(n.callee)
			for _, arg := range n.args {
				walkExpr(arg)
			}
		case *unaryExpr:
			walkExpr(n.expr)
		case *binaryExpr:
			walkExpr(n.left)
			walkExpr(n.right)
		case *blockExpr:
			for _, stmt := range n.stmts {
				walkStmtForGPU(stmt, walkExpr)
			}
			walkExpr(n.tail)
		case *tryExpr:
			walkExpr(n.body)
			walkExpr(n.handler)
		case *propagateExpr:
			walkExpr(n.expr)
		case *memberExpr:
			walkExpr(n.object)
		case *listExpr:
			for _, item := range n.items {
				walkExpr(item)
			}
		case *tupleExpr:
			for _, item := range n.items {
				walkExpr(item)
			}
		case *matchExpr:
			walkExpr(n.subject)
			for _, arm := range n.arms {
				walkExpr(arm.guard)
				walkExpr(arm.body)
			}
		}
	}
	for _, stmt := range prog.Statements {
		walkStmtForGPU(stmt, walkExpr)
		if unsupported != "" {
			break
		}
	}
	if unsupported != "" {
		return newGPUError(ErrGPUCapabilityUnavailable, "validate", "operation "+unsupported+" is outside the deterministic GPU profile", nil)
	}
	return nil
}

func walkStmtForGPU(stmt Stmt, walkExpr func(Expr)) {
	switch st := stmt.(type) {
	case letStmt:
		walkExpr(st.Expr)
	case assignStmt:
		walkExpr(st.Expr)
	case emitStmt:
		walkExpr(st.Expr)
	case macroStmt:
		walkExpr(st.Body)
	case funcStmt:
		walkExpr(st.Body)
	case returnStmt:
		walkExpr(st.Expr)
	case whileStmt:
		walkExpr(st.Cond)
		walkExpr(st.Body)
	case forStmt:
		walkExpr(st.Iter)
		walkExpr(st.Body)
	}
}

func gpuUnsupportedCall(name string) bool {
	switch name {
	case "bigint", "bigfloat", "complex", "rat", "rational", "fixed", "interval", "modint",
		"spawn", "await", "cancel", "parallel", "race", "select", "sleep", "read", "write",
		"tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "http_request", "file_open",
		"now", "random", "hash", "mix", "noise", "poly", "simd", "sparse_matrix", "sparse_tensor":
		return true
	default:
		return false
	}
}
