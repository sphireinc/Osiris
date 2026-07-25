# Sparse tensor and matrix design

Sparse values are a planned, separate numerical-storage family. They must not
reuse dense `tensor` values with sentinel zeroes, because that loses the
storage, ownership, and complexity contract users need for large problems.

## First public scope

The initial types will be `sparse_vector`, `sparse_matrix`, and
`sparse_tensor`. Each has an explicit shape, element family, format, ordered
index data, and values. Matrix construction accepts a declared format:
`sparse_matrix(rows, columns, entries[, format])`, where entries are
`[row, column, value]` triples and `format` is `csr` or `csc`.

Construction validates integer coordinates, bounds, finite shape product,
and duplicate entries. Duplicate coordinates are coalesced deterministically
in source order using the element-family addition rule; explicit stored zeroes
are removed. A canonical representation sorts indices lexicographically
(CSR by row then column; CSC by column then row), so printing, hashing,
serialization, and compiled artifacts are deterministic.

## Operations and conversions

`shape`, `at`, `setat`, `nnz`, `to_dense`, `to_sparse`, elementwise
operations with a zero-preserving scalar, sparse/dense matmul, and sparse
transpose are the first proposed operations. A non-zero-preserving operation
such as adding an arbitrary dense scalar must either materialize a dense
result or fail explicitly; it may never claim an unchanged sparse complexity.

Dense-to-sparse conversion has an explicit `zero`/tolerance policy. Exact
integer, decimal, and unit values compare exactly; floating values use a
caller-supplied non-negative tolerance, defaulting to exact zero. Complex,
dual, and future element families must define their zero and codec behavior
before they are admitted.

## Compiler and runtime gate

Inference may retain known shapes and the `sparse_*` family but must not claim
an `nnz` count after operations unless it is proven. The interpreter/VM codec
records format, shape, canonical indices, values, and element metadata.
Native lowering requires an owned ABI with an explicit allocator/deallocator,
overflow checks, aliasing rules, and executable parity tests. No sparse syntax
or public support claim is active until that gate is complete.
