#include <math.h>
#include <stdio.h>

static inline double gamma_v(double v, double c) {
  const double beta2 = (v * v) / (c * c);
  return 1.0 / sqrt(1.0 - beta2);
}

static inline double kinetic_energy(double v, double c, double mass) {
  return (gamma_v(v, c) - 1.0) * mass * c * c;
}

static double sweep_average(double steps) {
  const double c = 299792458.0;
  const double mass = 1.0;
  const double max_v = c * 0.99;
  const double dv = max_v / steps;

  double total = 0.0;
  double v = 0.0;

  for (int i = 0; i < (int)steps; ++i) {
    total += kinetic_energy(v, c, mass);
    v += dv;
  }

  return total / steps;
}

int main(void) {
  const double steps = 5000.0;
  printf("%.15f\n", sweep_average(steps));
  return 0;
}
