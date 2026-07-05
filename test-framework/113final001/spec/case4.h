#ifndef _CASE_H_
#define _CASE_H_
#define SECRET "f96597cf-d39d-4a97-a4c2-daab0500e76a"
#define DOCTEST_CONFIG_IMPLEMENT
#include <sstream>

#include "Complex.h"
#include "test.h"


TEST_CASE("Case4: Input stream and basic operations") {
  std::stringstream ss("x = 88 + 8.18*i");
  Complex c1;
  ss >> c1;
  CHECK_EQ(c1.real() , doctest::Approx(88));
  CHECK_EQ(c1.imag() , doctest::Approx(8.18));

  std::stringstream ss2("y = 5 + 6*i\nz = 10 + 10*i\n");
  Complex c2, c3;
  ss2 >> c2 >> c3;
  CHECK_EQ(c2 , Complex(5, 6));
  CHECK_EQ(c3 , Complex(10, 10));
  CHECK_EQ((c2 + c3) , Complex(15, 16));
  CHECK_EQ((c2 - c3) , Complex(-5, -4));
  CHECK_EQ((c2 * c3) , Complex(-10, 110));
  CHECK_EQ((c2 / c3).real() , doctest::Approx(0.55));
  CHECK_EQ((c2 / c3).imag() , doctest::Approx(0.05));
}

#endif // _CASE_H_
