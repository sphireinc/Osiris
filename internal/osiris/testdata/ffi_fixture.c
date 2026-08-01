#include <stdint.h>
#include <stdio.h>

int64_t osiris_ffi_add_i64(int64_t left, int64_t right) { return left + right; }
double osiris_ffi_mul_f64(double left, double right) { return left * right; }

const char *osiris_ffi_join_strings(const char *left, const char *right) {
    static char result[256];
    snprintf(result, sizeof(result), "%s%s", left ? left : "", right ? right : "");
    return result;
}

int64_t osiris_ffi_sum_buffer(const unsigned char *data, uintptr_t length) {
    int64_t total = 0;
    for (uintptr_t i = 0; i < length; i++) total += data[i];
    return total;
}
