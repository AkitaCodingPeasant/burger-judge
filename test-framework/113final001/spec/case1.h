#ifndef _CASE_H_
#define _CASE_H_
#define SECRET "29725514-8c54-4bae-8b78-3563eef2b55b"
#define DOCTEST_CONFIG_IMPLEMENT
#include "Complex.h"
#include "test.h"


TEST_CASE("Case1: Constructor test") {
  Complex c1;
  Complex c2(100);
  Complex c3(-200, 300);

  CHECK_EQ(c1.real(), doctest::Approx(0));
  CHECK_EQ(c1.imag(), doctest::Approx(0));
  CHECK_EQ(c2.real(), doctest::Approx(100));
  CHECK_EQ(c2.imag(), doctest::Approx(0));
  CHECK_EQ(c3.real(), doctest::Approx(-200));
  CHECK_EQ(c3.imag(), doctest::Approx(300));
}

#endif // _CASE_H_
