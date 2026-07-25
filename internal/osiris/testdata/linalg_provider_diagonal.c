#include <stddef.h>

typedef enum {
  O_LINALG_OK = 0,
  O_LINALG_INVALID_ARGUMENT,
  O_LINALG_NOT_CONVERGED,
  O_LINALG_NO_MEMORY,
  O_LINALG_UNAVAILABLE
} OLinalgStatus;

typedef struct {
  double real;
  double imag;
} OComplex;

typedef struct {
  unsigned abi_version;
  OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *,
                           double *);
} OLinalgProviderV1;

typedef struct {
  unsigned abi_version;
  OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *,
                           double *);
  OLinalgStatus (*eig_f64)(size_t, double *, OComplex *, OComplex *);
} OLinalgProviderV2;

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

static OLinalgStatus diagonal_eig(size_t n, double *matrix, OComplex *values,
                                  OComplex *vectors) {
  if (n == 2 && matrix[0] == 0.0 && matrix[1] == -1.0 &&
      matrix[2] == 1.0 && matrix[3] == 0.0) {
    /* A 90-degree rotation: eigenvalues +i and -i, column eigenvectors
       [1, -i] and [1, i]. This is an ABI/residual fixture, not a solver. */
    values[0] = (OComplex){0.0, 1.0};
    values[1] = (OComplex){0.0, -1.0};
    vectors[0] = (OComplex){1.0, 0.0};
    vectors[1] = (OComplex){1.0, 0.0};
    vectors[2] = (OComplex){0.0, -1.0};
    vectors[3] = (OComplex){0.0, 1.0};
    return O_LINALG_OK;
  }
  for (size_t row = 0; row < n; row++) {
    for (size_t col = 0; col < n; col++) {
      if (row != col && matrix[row * n + col] != 0.0) {
        return O_LINALG_UNAVAILABLE;
      }
    }
  }
  for (size_t row = 0; row < n; row++) {
    values[row] = (OComplex){matrix[row * n + row], 0.0};
    for (size_t col = 0; col < n; col++) {
      vectors[row * n + col] = (OComplex){row == col ? 1.0 : 0.0, 0.0};
    }
  }
  return O_LINALG_OK;
}

static const OLinalgProviderV2 provider = {
  2u,
  diagonal_eigh,
  diagonal_svd,
  diagonal_eig,
};

const OLinalgProviderV1 *osiris_linalg_provider_v1(void) {
  return (const OLinalgProviderV1 *)&provider;
}
