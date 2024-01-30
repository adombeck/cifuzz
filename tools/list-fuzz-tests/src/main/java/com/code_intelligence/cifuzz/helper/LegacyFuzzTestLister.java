package com.code_intelligence.cifuzz.helper;

import io.github.classgraph.ClassGraph;
import io.github.classgraph.ClassInfo;
import io.github.classgraph.ScanResult;
import java.util.List;
import java.util.stream.Collectors;

final class LegacyFuzzTestLister {

  public static List<String> listFuzzTests(List<String> classes) {
    ClassGraph scanConfig =
        new ClassGraph()
            // Calling acceptClasses with an empty array is a no-op, so we can safely call it even
            // if classes is empty and we should scan all classes.
            .acceptClasses(classes.toArray(new String[0]))
            // Only scan directories, not .jar files, as with Maven and Gradle the project's own
            // classes are typically contained in a directory, and we do not want to scan
            // third-party dependencies for fuzz tests.
            .disableJarScanning()
            .enableMethodInfo();
    try (ScanResult result = scanConfig.scan()) {
      return result.getAllClasses().stream()
          .filter(LegacyFuzzTestLister::hasFuzzerTestOneInput)
          .map(ClassInfo::getName)
          .sorted()
          .collect(Collectors.toList());
    }
  }

  private static boolean hasFuzzerTestOneInput(ClassInfo classInfo) {
    return !classInfo
        .getDeclaredMethodInfo("fuzzerTestOneInput")
        .filter(methodInfo -> methodInfo.isStatic() && methodInfo.isPublic())
        .isEmpty();
  }

  private LegacyFuzzTestLister() {}
}
