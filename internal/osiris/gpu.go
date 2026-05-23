package osiris

import (
	"context"
	"encoding/json"
	"strings"
)

type GPUKernel struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type GPUArtifact struct {
	Source  string      `json:"source"`
	Kernels []GPUKernel `json:"kernels"`
}

// CompileGPU compiles the gpu into the requested backend artifact so the caller can execute or inspect the program without keeping the source interpreter in the hot path.. It returns GPUArtifact and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func CompileGPU(ctx context.Context, prog *Program) (GPUArtifact, error) {
	_ = ctx
	plan := collectGPUPlan(prog)
	return GPUArtifact{
		Source:  renderGPUOpenCL(plan),
		Kernels: plan.Kernels,
	}, nil
}

type gpuPlan struct {
	Kernels []GPUKernel
	seen    map[string]struct{}
}

// collectGPUPlan performs the collect gpu plan step in the surrounding package and returns gpuPlan.. It returns gpuPlan so callers can continue with the next stage of the pipeline without re-deriving the same information.
func collectGPUPlan(prog *Program) gpuPlan {
	plan := gpuPlan{seen: make(map[string]struct{})}
	for _, stmt := range prog.Statements {
		plan.walkStmt(stmt)
	}
	if len(plan.Kernels) == 0 {
		plan.addKernel("osiris_elemwise_add", "elementwise")
		plan.addKernel("osiris_elemwise_sub", "elementwise")
		plan.addKernel("osiris_elemwise_mul", "elementwise")
		plan.addKernel("osiris_elemwise_div", "elementwise")
		plan.addKernel("osiris_dot", "dot")
		plan.addKernel("osiris_matmul", "matmul")
	}
	return plan
}

// addKernel handles on gpuPlan the add kernel. It keeps the gpuPlan subsystem self-contained by handling the add kernel-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *gpuPlan) addKernel(name, kind string) {
	key := name + "::" + kind
	if _, ok := p.seen[key]; ok {
		return
	}
	p.seen[key] = struct{}{}
	p.Kernels = append(p.Kernels, GPUKernel{Name: name, Kind: kind})
}

// walkStmt handles on gpuPlan the walk stmt. It keeps the gpuPlan subsystem self-contained by handling the walk stmt-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *gpuPlan) walkStmt(stmt Stmt) {
	switch st := stmt.(type) {
	case letStmt:
		p.walkExpr(st.Expr)
	case emitStmt:
		p.walkExpr(st.Expr)
	case macroStmt:
		p.walkExpr(st.Body)
	case funcStmt:
		p.walkExpr(st.Body)
	case returnStmt:
		p.walkExpr(st.Expr)
	case whileStmt:
		p.walkExpr(st.Cond)
		p.walkExpr(st.Body)
	case forStmt:
		p.walkExpr(st.Iter)
		p.walkExpr(st.Body)
	case importStmt, packageStmt, exportStmt, breakStmt, continueStmt:
		return
	}
}

// walkExpr handles on gpuPlan the walk expr. It keeps the gpuPlan subsystem self-contained by handling the walk expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *gpuPlan) walkExpr(expr Expr) {
	switch n := expr.(type) {
	case nil:
		return
	case *literalExpr, *identExpr:
		return
	case *unaryExpr:
		p.walkExpr(n.expr)
	case *binaryExpr:
		p.walkExpr(n.left)
		p.walkExpr(n.right)
		switch n.op {
		case tokenPlus:
			p.addKernel("osiris_elemwise_add", "elementwise")
		case tokenMinus:
			p.addKernel("osiris_elemwise_sub", "elementwise")
		case tokenStar:
			p.addKernel("osiris_elemwise_mul", "elementwise")
		case tokenSlash:
			p.addKernel("osiris_elemwise_div", "elementwise")
		}
	case *callExpr:
		p.walkExpr(n.callee)
		for _, arg := range n.args {
			p.walkExpr(arg)
		}
		switch callName(n.callee) {
		case "dot":
			p.addKernel("osiris_dot", "dot")
		case "matmul":
			p.addKernel("osiris_matmul", "matmul")
		case "tensor", "matrix", "reshape", "transpose", "vector", "vec":
			p.addKernel("osiris_shape_transform", "shape")
		}
	case *blockExpr:
		for _, stmt := range n.stmts {
			p.walkStmt(stmt)
		}
		p.walkExpr(n.tail)
	case *tryExpr:
		p.walkExpr(n.body)
		p.walkExpr(n.handler)
	case *memberExpr:
		p.walkExpr(n.object)
	case *listExpr:
		for _, item := range n.items {
			p.walkExpr(item)
		}
	case *matchExpr:
		p.walkExpr(n.subject)
		for _, arm := range n.arms {
			p.walkExpr(arm.body)
		}
	}
}

// callName performs the call name step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func callName(expr Expr) string {
	id, ok := expr.(*identExpr)
	if !ok {
		return ""
	}
	return id.name
}

// renderGPUOpenCL performs the render gpu open cl step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func renderGPUOpenCL(plan gpuPlan) string {
	var b strings.Builder
	b.WriteString("__kernel void osiris_elemwise_add(__global const double* lhs, __global const double* rhs, __global double* out, ulong n) {\n")
	b.WriteString("  ulong i = get_global_id(0);\n")
	b.WriteString("  if (i >= n) return;\n")
	b.WriteString("  out[i] = lhs[i] + rhs[i];\n}\n\n")
	b.WriteString("__kernel void osiris_elemwise_sub(__global const double* lhs, __global const double* rhs, __global double* out, ulong n) {\n")
	b.WriteString("  ulong i = get_global_id(0);\n")
	b.WriteString("  if (i >= n) return;\n")
	b.WriteString("  out[i] = lhs[i] - rhs[i];\n}\n\n")
	b.WriteString("__kernel void osiris_elemwise_mul(__global const double* lhs, __global const double* rhs, __global double* out, ulong n) {\n")
	b.WriteString("  ulong i = get_global_id(0);\n")
	b.WriteString("  if (i >= n) return;\n")
	b.WriteString("  out[i] = lhs[i] * rhs[i];\n}\n\n")
	b.WriteString("__kernel void osiris_elemwise_div(__global const double* lhs, __global const double* rhs, __global double* out, ulong n) {\n")
	b.WriteString("  ulong i = get_global_id(0);\n")
	b.WriteString("  if (i >= n) return;\n")
	b.WriteString("  out[i] = lhs[i] / rhs[i];\n}\n\n")
	b.WriteString("__kernel void osiris_dot(__global const double* lhs, __global const double* rhs, __global double* out, ulong n) {\n")
	b.WriteString("  ulong i = get_global_id(0);\n")
	b.WriteString("  if (i != 0) return;\n")
	b.WriteString("  double acc = 0.0;\n")
	b.WriteString("  for (ulong j = 0; j < n; ++j) acc += lhs[j] * rhs[j];\n")
	b.WriteString("  out[0] = acc;\n}\n\n")
	b.WriteString("__kernel void osiris_matmul(__global const double* left, __global const double* right, __global double* out, ulong rows, ulong mid, ulong cols) {\n")
	b.WriteString("  ulong r = get_global_id(0);\n")
	b.WriteString("  ulong c = get_global_id(1);\n")
	b.WriteString("  if (r >= rows || c >= cols) return;\n")
	b.WriteString("  double acc = 0.0;\n")
	b.WriteString("  for (ulong k = 0; k < mid; ++k) acc += left[r * mid + k] * right[k * cols + c];\n")
	b.WriteString("  out[r * cols + c] = acc;\n}\n\n")
	if len(plan.Kernels) > 0 {
		b.WriteString("// kernels requested by program:\n")
		for _, k := range plan.Kernels {
			b.WriteString("// ")
			b.WriteString(k.Kind)
			b.WriteString(": ")
			b.WriteString(k.Name)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// EncodeGPUArtifactJSON performs the encode gpu artifact json step in the surrounding package and returns []byte, error.. It returns []byte and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func EncodeGPUArtifactJSON(art GPUArtifact) ([]byte, error) {
	return json.MarshalIndent(art, "", "  ")
}
