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
  OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *, double *);
} OLinalgProviderV1;

static const OLinalgProviderV1 incompatible_provider = {0u, NULL, NULL};

const OLinalgProviderV1 *osiris_linalg_provider_v1(void) {
  return &incompatible_provider;
}
