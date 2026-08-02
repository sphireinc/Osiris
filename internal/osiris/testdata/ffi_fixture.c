#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

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

int64_t osiris_ffi_increment_buffer(unsigned char *data, uintptr_t length) {
    for (uintptr_t i = 0; i < length; i++) data[i]++;
    return (int64_t)length;
}

int64_t osiris_ffi_sum_point(const unsigned char *data, uintptr_t length) {
    if (!data || length < 8) return -1;
    int32_t x = 0, y = 0;
    memcpy(&x, data, sizeof(x));
    memcpy(&y, data + 4, sizeof(y));
    return (int64_t)x + (int64_t)y;
}

void *osiris_ffi_create_handle(void *ignored) {
    (void)ignored;
    return malloc(8);
}

void osiris_ffi_release_handle(void *value) {
    free(value);
}

int64_t osiris_ffi_invoke_callback(int64_t (*callback)(uintptr_t, int64_t), uintptr_t id) {
    return callback(id, 41);
}
