package com.code_intelligence.cifuzz.helper.some_package;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

public class SomeTests {

  @Test
  void someUnitTest() {}

  @FuzzTest(maxDuration = "10m")
  void myTest(byte[] bytes) {}

  @FuzzTest
  @DisplayName("I am a fuzz test")
  void myDisplayNameFuzz(FuzzedDataProvider data) {}

  @Test
  void anotherUnitTest() {}

  @FuzzTest
  void autofuzzTest(String foo, int bar) {}

  public static void fuzzerTestOneInput(FuzzedDataProvider data) {}
}
