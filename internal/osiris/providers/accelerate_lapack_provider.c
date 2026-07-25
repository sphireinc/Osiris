/*
 * Optional production Osiris numerical-provider adapter for Apple Accelerate.
 * Build with: -linalg-provider internal/osiris/providers/accelerate_lapack_provider.c
 *             -linalg-provider-flag=-framework
 *             -linalg-provider-flag=Accelerate
 *
 * This file deliberately contains only the ABI adaptation layer; Accelerate
 * supplies the licensed, optimized LAPACK implementation at link time.
 */
#include <Accelerate/Accelerate.h>
#include <limits.h>
#include <stddef.h>
#include <stdlib.h>

typedef enum {
  O_LINALG_OK = 0, O_LINALG_INVALID_ARGUMENT, O_LINALG_NOT_CONVERGED,
  O_LINALG_NO_MEMORY, O_LINALG_UNAVAILABLE
} OLinalgStatus;

typedef struct { double real; double imag; } OComplex;
typedef struct {
  unsigned abi_version;
  OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *, double *);
} OLinalgProviderV1;
typedef struct {
  unsigned abi_version;
  OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *, double *);
  OLinalgStatus (*eig_f64)(size_t, double *, OComplex *, OComplex *);
} OLinalgProviderV2;

static int osiris_lapack_size(size_t value, __CLPK_integer *out) {
  if (value > (size_t)LONG_MAX) return 0;
  *out = (__CLPK_integer)value;
  return 1;
}

static double *osiris_column_major(const double *row_major, size_t rows, size_t columns) {
  if (rows && columns > SIZE_MAX / rows) return NULL;
  size_t count = rows * columns;
  double *column_major = count ? (double *)malloc(count * sizeof(double)) : NULL;
  if (count && !column_major) return NULL;
  for (size_t row = 0; row < rows; row++)
    for (size_t column = 0; column < columns; column++)
      column_major[row + column * rows] = row_major[row * columns + column];
  return column_major;
}

static void osiris_row_major(double *row_major, const double *column_major, size_t rows, size_t columns) {
  for (size_t row = 0; row < rows; row++)
    for (size_t column = 0; column < columns; column++)
      row_major[row * columns + column] = column_major[row + column * rows];
}

static OLinalgStatus accelerate_eigh(size_t n, double *matrix, double *values, double *vectors) {
  __CLPK_integer size;
  if (!osiris_lapack_size(n, &size)) return O_LINALG_INVALID_ARGUMENT;
  if (n == 0) return O_LINALG_OK;
  double *a = osiris_column_major(matrix, n, n);
  if (!a) return O_LINALG_NO_MEMORY;
  char jobz = 'V', uplo = 'U';
  __CLPK_integer info = 0, lwork = -1, liwork = -1;
  double work_query;
  __CLPK_integer iwork_query;
  dsyevd_(&jobz, &uplo, &size, a, &size, values, &work_query, &lwork,
          &iwork_query, &liwork, &info);
  if (info != 0 || work_query > (double)LONG_MAX || iwork_query < 1) {
    free(a);
    return info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT;
  }
  lwork = (__CLPK_integer)work_query;
  liwork = iwork_query;
  double *work = (double *)malloc((size_t)lwork * sizeof(double));
  __CLPK_integer *iwork = (__CLPK_integer *)malloc((size_t)liwork * sizeof(__CLPK_integer));
  if (!work || !iwork) { free(a); free(work); free(iwork); return O_LINALG_NO_MEMORY; }
  dsyevd_(&jobz, &uplo, &size, a, &size, values, work, &lwork, iwork, &liwork, &info);
  if (info == 0) osiris_row_major(vectors, a, n, n);
  free(a); free(work); free(iwork);
  return info == 0 ? O_LINALG_OK : (info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT);
}

static OLinalgStatus accelerate_svd(size_t rows, size_t columns, double *matrix,
                                    double *u, double *values, double *vt) {
  __CLPK_integer m, n;
  if (!osiris_lapack_size(rows, &m) || !osiris_lapack_size(columns, &n) || rows < columns) return O_LINALG_INVALID_ARGUMENT;
  if (columns == 0) return O_LINALG_OK;
  double *a = osiris_column_major(matrix, rows, columns);
  if (!a) return O_LINALG_NO_MEMORY;
  size_t u_count = rows * columns, vt_count = columns * columns;
  double *u_col = (double *)malloc(u_count * sizeof(double));
  double *vt_col = (double *)malloc(vt_count * sizeof(double));
  if (!u_col || !vt_col) { free(a); free(u_col); free(vt_col); return O_LINALG_NO_MEMORY; }
  char jobz = 'S'; __CLPK_integer info = 0, lwork = -1; double work_query;
  __CLPK_integer *iwork = (__CLPK_integer *)malloc((size_t)(8 * columns) * sizeof(__CLPK_integer));
  if (!iwork) { free(a); free(u_col); free(vt_col); return O_LINALG_NO_MEMORY; }
  dgesdd_(&jobz, &m, &n, a, &m, values, u_col, &m, vt_col, &n, &work_query, &lwork, iwork, &info);
  if (info != 0 || work_query > (double)LONG_MAX) { free(a); free(u_col); free(vt_col); free(iwork); return info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT; }
  lwork = (__CLPK_integer)work_query;
  double *work = (double *)malloc((size_t)lwork * sizeof(double));
  if (!work) { free(a); free(u_col); free(vt_col); free(iwork); return O_LINALG_NO_MEMORY; }
  dgesdd_(&jobz, &m, &n, a, &m, values, u_col, &m, vt_col, &n, work, &lwork, iwork, &info);
  if (info == 0) { osiris_row_major(u, u_col, rows, columns); osiris_row_major(vt, vt_col, columns, columns); }
  free(a); free(u_col); free(vt_col); free(iwork); free(work);
  return info == 0 ? O_LINALG_OK : (info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT);
}

static OLinalgStatus accelerate_eig(size_t n, double *matrix, OComplex *values, OComplex *vectors) {
  __CLPK_integer size;
  if (!osiris_lapack_size(n, &size)) return O_LINALG_INVALID_ARGUMENT;
  if (n == 0) return O_LINALG_OK;
  double *a = osiris_column_major(matrix, n, n), *wr = (double *)malloc(n * sizeof(double)), *wi = (double *)malloc(n * sizeof(double)), *vr = (double *)malloc(n * n * sizeof(double));
  if (!a || !wr || !wi || !vr) { free(a); free(wr); free(wi); free(vr); return O_LINALG_NO_MEMORY; }
  char jobvl = 'N', jobvr = 'V'; __CLPK_integer info = 0, lwork = -1; double work_query, unused_left;
  dgeev_(&jobvl, &jobvr, &size, a, &size, wr, wi, &unused_left, &size, vr, &size, &work_query, &lwork, &info);
  if (info != 0 || work_query > (double)LONG_MAX) { free(a); free(wr); free(wi); free(vr); return info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT; }
  lwork = (__CLPK_integer)work_query; double *work = (double *)malloc((size_t)lwork * sizeof(double));
  if (!work) { free(a); free(wr); free(wi); free(vr); return O_LINALG_NO_MEMORY; }
  dgeev_(&jobvl, &jobvr, &size, a, &size, wr, wi, &unused_left, &size, vr, &size, work, &lwork, &info);
  if (info == 0) for (size_t column = 0; column < n; column++) {
    values[column] = (OComplex){wr[column], wi[column]};
    for (size_t row = 0; row < n; row++) {
      double real = vr[row + column * n], imag = 0;
      if (wi[column] > 0 && column + 1 < n) imag = vr[row + (column + 1) * n];
      else if (wi[column] < 0 && column > 0) {
        real = vr[row + (column - 1) * n];
        imag = -vr[row + column * n];
      }
      vectors[row * n + column] = (OComplex){real, imag};
    }
  }
  free(a); free(wr); free(wi); free(vr); free(work);
  return info == 0 ? O_LINALG_OK : (info > 0 ? O_LINALG_NOT_CONVERGED : O_LINALG_INVALID_ARGUMENT);
}

static const OLinalgProviderV2 accelerate_provider = {2u, accelerate_eigh, accelerate_svd, accelerate_eig};
const OLinalgProviderV1 *osiris_linalg_provider_v1(void) { return (const OLinalgProviderV1 *)&accelerate_provider; }
