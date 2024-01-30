plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.8.0"
}

sourceSets.getByName("test") {
	java.srcDir("junit-src")
}

repositories {
    // Use Maven Central for resolving dependencies.
    mavenCentral()
}

dependencies {
	implementation(project(":app"))
	testImplementation(platform("org.junit:junit-bom:5.10.0"))
	testImplementation("org.junit.jupiter:junit-jupiter")
  testImplementation("com.code-intelligence:jazzer-junit:0.21.1")
}

tasks.test {
	useJUnitPlatform()
	testLogging {
		events("passed", "skipped", "failed")
	}
}

