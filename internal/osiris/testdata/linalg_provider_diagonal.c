#include <stddef.h>

typedef enum {
  O_LINALG_OK = 0,
  O_LINALG_INVALID_ARGUMENT,
  O_LINALG_NOT_CONVERGED,
  O_LINALG_NO_MEMORY,
  O_LINALG_UNAVAILABLE
} OLinalgStatus;

typedef struct {
  unsigned abi_version;
  OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *,
                           double *);
} OLinalgProviderV1;

static OLinalgStatus diagonal_eigh(size_t n, double *matrix, double *values,
                                   double *vectors) {
  for (size_t row = 0; row < n; row++) {
    values[row] = matrix[row * n + row];
    for (size_t col = 0; col < n; col++) {
      vectors[row * n + col] = row == col ? 1.0 : 0.0;
    }
  }
  return O_LINALG_OK;
}

static OLinalgStatus diagonal_svd(size_t rows, size_t columns, double *matrix,
                                  double *u, double *values, double *vt) {
  for (size_t row = 0; row < rows; row++) {
    for (size_t col = 0; col < columns; col++) {
      u[row * columns + col] = row == col ? 1.0 : 0.0;
    }
  }
  for (size_t row = 0; row < columns; row++) {
    values[row] = matrix[row * columns + row];
    for (size_t col = 0; col < columns; col++) {
      vt[row * columns + col] = row == col ? 1.0 : 0.0;
    }
  }
  return O_LINALG_OK;
}

static const OLinalgProviderV1 provider = {
  1u,
  diagonal_eigh,
  diagonal_svd,
};

const OLinalgProviderV1 *osiris_linalg_provider_v1(void) { return &provider; }
