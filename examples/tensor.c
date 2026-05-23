#include <stdio.h>

// Osiris C snapshot from examples/tensor.osr
static const char *osiris_outputs[] = {
  "9223372036854775807",
};
static const int osiris_output_count = 1;

int main(void) {
  for (int i = 0; i < osiris_output_count; ++i) {
    puts(osiris_outputs[i]);
  }
  return 0;
}
