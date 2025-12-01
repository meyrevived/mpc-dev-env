plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.20"
    id("org.jetbrains.intellij") version "1.16.0"
    id("org.jetbrains.kotlin.plugin.serialization") version "1.9.20"
}

group = "com.redhat"
version = "0.1.0"

repositories {
    mavenCentral()
}

dependencies {
    // Ktor HTTP client - exclude SLF4J to avoid classloader conflicts with IntelliJ Platform
    // Note: We exclude coroutines from Ktor and let IntelliJ Platform provide it
    implementation("io.ktor:ktor-client-core:2.3.5") {
        exclude(group = "org.slf4j")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-core")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-jdk8")
    }
    implementation("io.ktor:ktor-client-cio:2.3.5") {
        exclude(group = "org.slf4j")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-core")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-jdk8")
    }
    implementation("io.ktor:ktor-client-websockets:2.3.5") {
        exclude(group = "org.slf4j")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-core")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-jdk8")
    }
    implementation("io.ktor:ktor-client-content-negotiation:2.3.5") {
        exclude(group = "org.slf4j")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-core")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-jdk8")
    }
    implementation("io.ktor:ktor-serialization-kotlinx-json:2.3.5") {
        exclude(group = "org.slf4j")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-core")
        exclude(group = "org.jetbrains.kotlinx", module = "kotlinx-coroutines-jdk8")
    }

    // Explicitly use the coroutines version provided by IntelliJ Platform
    // This is marked as compileOnly so it won't be bundled, but Ktor can compile against it
    compileOnly("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.7.3")
    compileOnly("org.jetbrains.kotlinx:kotlinx-coroutines-jdk8:1.7.3")

    // Kotlin serialization
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.0")

    // Testing
    testImplementation("org.jetbrains.kotlin:kotlin-test")
    testImplementation("io.ktor:ktor-client-mock:2.3.5")
    testImplementation("io.mockk:mockk:1.13.8")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.7.3")
}

intellij {
    version.set("2023.2")
    type.set("GO")
    plugins.set(listOf("org.jetbrains.plugins.go"))
}

tasks {
    withType<JavaCompile> {
        sourceCompatibility = "17"
        targetCompatibility = "17"
    }

    withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile> {
        kotlinOptions.jvmTarget = "17"
    }

    patchPluginXml {
        sinceBuild.set("232")
        untilBuild.set("252.*")
    }

    signPlugin {
        certificateChain.set(System.getenv("CERTIFICATE_CHAIN"))
        privateKey.set(System.getenv("PRIVATE_KEY"))
        password.set(System.getenv("PRIVATE_KEY_PASSWORD"))
    }

    publishPlugin {
        token.set(System.getenv("PUBLISH_TOKEN"))
    }
}
