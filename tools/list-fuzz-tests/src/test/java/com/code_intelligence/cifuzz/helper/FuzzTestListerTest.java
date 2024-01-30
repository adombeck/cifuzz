package com.code_intelligence.cifuzz.helper;

import static com.google.common.truth.Truth.assertThat;
import static com.google.common.truth.Truth8.assertThat;
import static java.util.Arrays.asList;
import static java.util.Collections.emptyList;

import java.util.List;
import java.util.Optional;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

public class FuzzTestListerTest {

  @Test
  @Timeout(10)
  void listAllJUnitFuzzTests() {
    Optional<List<String>> fuzzTests = JUnitFuzzTestLister.listFuzzTests(emptyList());

    assertThat(fuzzTests).isPresent();
    assertThat(fuzzTests.get())
        .containsExactly(
            "com.code_intelligence.cifuzz.helper.some_package.EvenMoreTests::autofuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.EvenMoreTests::myDisplayNameFuzz",
            "com.code_intelligence.cifuzz.helper.some_package.EvenMoreTests::myTest",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::autofuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::myDisplayNameFuzz",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::myTest",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::autofuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::myDisplayNameFuzz",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::myTest")
        .inOrder();
  }

  @Test
  void listJUnitFuzzTestsInSpecificClasses() {
    Optional<List<String>> fuzzTests =
        JUnitFuzzTestLister.listFuzzTests(
            asList(
                "com.code_intelligence.cifuzz.helper.some_package.NoTests",
                "com.code_intelligence.cifuzz.helper.some_package.SomeTests",
                "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests"));

    assertThat(fuzzTests).isPresent();
    assertThat(fuzzTests.get())
        .containsExactly(
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::autofuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::myDisplayNameFuzz",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests::myTest",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::autofuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::myDisplayNameFuzz",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests::myTest")
        .inOrder();
  }

  @Test
  void listAllLegacyFuzzTests() {
    List<String> fuzzTests = LegacyFuzzTestLister.listFuzzTests(emptyList());

    assertThat(fuzzTests)
        .containsExactly(
            "com.code_intelligence.cifuzz.helper.some_package.LegacyFuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.SomeTests",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests")
        .inOrder();
  }

  @Test
  void listLegacyFuzzTestsInSpecificClasses() {
    List<String> fuzzTests =
        LegacyFuzzTestLister.listFuzzTests(
            asList(
                "com.code_intelligence.cifuzz.helper.some_package.LegacyFuzzTest",
                "com.code_intelligence.cifuzz.helper.some_package.NoTests",
                "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests"));

    assertThat(fuzzTests)
        .containsExactly(
            "com.code_intelligence.cifuzz.helper.some_package.LegacyFuzzTest",
            "com.code_intelligence.cifuzz.helper.some_package.sub_package.SomeMoreTests")
        .inOrder();
  }
}
