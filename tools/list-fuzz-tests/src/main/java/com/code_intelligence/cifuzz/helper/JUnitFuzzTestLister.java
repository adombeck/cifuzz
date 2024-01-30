package com.code_intelligence.cifuzz.helper;

import static java.util.Arrays.asList;
import static java.util.Collections.unmodifiableList;
import static java.util.stream.Collectors.toList;
import static org.junit.platform.launcher.EngineFilter.includeEngines;
import static org.junit.platform.launcher.TagFilter.includeTags;

import java.io.File;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.List;
import java.util.Optional;
import java.util.regex.Pattern;
import java.util.stream.Collectors;
import org.junit.platform.engine.DiscoverySelector;
import org.junit.platform.engine.UniqueId.Segment;
import org.junit.platform.engine.discovery.DiscoverySelectors;
import org.junit.platform.launcher.LauncherDiscoveryRequest;
import org.junit.platform.launcher.TestIdentifier;
import org.junit.platform.launcher.TestPlan;
import org.junit.platform.launcher.core.LauncherDiscoveryRequestBuilder;
import org.junit.platform.launcher.core.LauncherFactory;

final class JUnitFuzzTestLister {

  private static final Pattern CLASSPATH_SPLITTER =
      Pattern.compile(Pattern.quote(File.pathSeparator));

  public static Optional<List<String>> listFuzzTests(List<String> classes) {
    if (!isJUnitOnClasspath()) {
      return Optional.empty();
    }

    LauncherDiscoveryRequest request =
        LauncherDiscoveryRequestBuilder.request()
            .selectors(selectorsFor(classes))
            .filters(
                includeEngines("junit-jupiter"),
                // All @FuzzTests are annotated with this tag.
                includeTags("jazzer"))
            .build();
    TestPlan testPlan = LauncherFactory.create().discover(request);
    return Optional.of(
        testPlan
            // Test engine level
            .getRoots()
            .stream()
            // Test class level
            .flatMap(engineTestIdentifier -> testPlan.getDescendants(engineTestIdentifier).stream())
            // Test method level
            .flatMap(classTestIdentifier -> testPlan.getDescendants(classTestIdentifier).stream())
            .map(JUnitFuzzTestLister::toMethodReference)
            .filter(Optional::isPresent)
            .map(Optional::get)
            .sorted()
            // Jazzer only runs a single fuzz test per method name, the one that comes first in
            // JUnit
            // execution order.
            // TODO: Clarify whether it would be better to error out in case of duplicates. Whereas
            //       the fuzz tests that aren't executed during a JUnit test run are clearly marked
            //       as skipped, this may be less visible during a remote run.
            .distinct()
            .collect(toList()));
  }

  private static boolean isJUnitOnClasspath() {
    try {
      Class.forName("org.junit.platform.launcher.LauncherDiscoveryRequest");
      Class.forName("org.junit.platform.engine.DiscoverySelector");
      return true;
    } catch (ClassNotFoundException e) {
      return false;
    }
  }

  private static List<? extends DiscoverySelector> selectorsFor(List<String> classes) {
    if (classes.isEmpty()) {
      return DiscoverySelectors.selectClasspathRoots(
          CLASSPATH_SPLITTER
              .splitAsStream(System.getProperty("java.class.path"))
              .map(Paths::get)
              // Only scan directories, not .jar files, as with Maven and Gradle the project's own
              // classes are typically contained in a directory, and we do not want to scan
              // third-party dependencies for fuzz tests.
              .filter(Files::isDirectory)
              .collect(Collectors.toSet()));
    }
    return classes.stream().map(DiscoverySelectors::selectClass).collect(toList());
  }

  // @FuzzTest unique IDs are expected to be of the form:
  // [engine:junit-jupiter]/[class:com.example.MyTests]/[test-template:myFuzzTest(com.code_intelligence.jazzer.api.FuzzedDataProvider)]
  private static final List<String> EXPECTED_SEGMENT_TYPES =
      unmodifiableList(asList("engine", "class", "test-template"));

  private static Optional<String> toMethodReference(TestIdentifier testIdentifier) {
    List<Segment> segments = testIdentifier.getUniqueIdObject().getSegments();
    if (!segments.stream().map(Segment::getType).collect(toList()).equals(EXPECTED_SEGMENT_TYPES)) {
      return Optional.empty();
    }

    String className = segments.get(1).getValue();
    String methodNameAndArgs = segments.get(2).getValue();
    String methodName = methodNameAndArgs.substring(0, methodNameAndArgs.indexOf('('));
    return Optional.of(String.format("%s::%s", className, methodName));
  }

  private JUnitFuzzTestLister() {}
}
