import groovy.json.JsonSlurper
import java.util.Properties

plugins {
    id("com.android.application")
}

android {
    namespace = "com.kloneets.kokotools"
    compileSdk = 36

    val keystorePropertiesFile = rootProject.file("keystore.properties")
    val keystoreProperties = Properties()
    if (keystorePropertiesFile.isFile) {
        keystorePropertiesFile.inputStream().use { keystoreProperties.load(it) }
    }

    fun googleServicesConfig(): Map<String, String> {
        val file = listOf(
            rootProject.file("google-services.json"),
            project.file("google-services.json"),
        ).firstOrNull { it.isFile } ?: return emptyMap()
        val root = JsonSlurper().parse(file) as Map<*, *>
        val projectInfo = root["project_info"] as? Map<*, *> ?: emptyMap<Any, Any>()
        val client = (root["client"] as? List<*>)?.firstOrNull() as? Map<*, *> ?: emptyMap<Any, Any>()
        val apiKey = (client["api_key"] as? List<*>)?.firstOrNull() as? Map<*, *> ?: emptyMap<Any, Any>()
        val webOAuthClient = (client["oauth_client"] as? List<*>)
            ?.mapNotNull { it as? Map<*, *> }
            ?.firstOrNull { (it["client_type"] as? Number)?.toInt() == 3 }
            ?: emptyMap<Any, Any>()
        return mapOf(
            "KOKO_FIREBASE_API_KEY" to apiKey["current_key"].orEmptyString(),
            "KOKO_FIREBASE_DATABASE_URL" to projectInfo["firebase_url"].orEmptyString(),
            "KOKO_FIREBASE_PROJECT_ID" to projectInfo["project_id"].orEmptyString(),
            "KOKO_GOOGLE_WEB_CLIENT_ID" to webOAuthClient["client_id"].orEmptyString(),
        )
    }

    val googleServicesConfig = googleServicesConfig()

    fun firebaseBuildConfigValue(name: String): String {
        return providers.gradleProperty(name).orElse(googleServicesConfig[name].orEmpty()).get()
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
    }

    defaultConfig {
        applicationId = "com.kloneets.kokotools"
        minSdk = 26
        targetSdk = 35
        versionCode = 2
        versionName = "0.1.1"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        buildConfigField("String", "FIREBASE_API_KEY", "\"${firebaseBuildConfigValue("KOKO_FIREBASE_API_KEY")}\"")
        buildConfigField("String", "FIREBASE_DATABASE_URL", "\"${firebaseBuildConfigValue("KOKO_FIREBASE_DATABASE_URL")}\"")
        buildConfigField("String", "FIREBASE_PROJECT_ID", "\"${firebaseBuildConfigValue("KOKO_FIREBASE_PROJECT_ID")}\"")
        buildConfigField("String", "GOOGLE_WEB_CLIENT_ID", "\"${firebaseBuildConfigValue("KOKO_GOOGLE_WEB_CLIENT_ID")}\"")

        externalNativeBuild {
            cmake {
                cppFlags += listOf("-std=c++17")
            }
        }
    }

    signingConfigs {
        create("release") {
            if (keystorePropertiesFile.isFile) {
                storeFile = rootProject.file(keystoreProperties.getProperty("storeFile"))
                storePassword = keystoreProperties.getProperty("storePassword")
                keyAlias = keystoreProperties.getProperty("keyAlias")
                keyPassword = keystoreProperties.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            if (keystorePropertiesFile.isFile) {
                signingConfig = signingConfigs.getByName("release")
            }
            isMinifyEnabled = false
        }
    }

    buildFeatures {
        buildConfig = true
    }

    sourceSets {
        getByName("test") {
            resources.directories.add("../../src/sync/testdata")
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
}

fun Any?.orEmptyString(): String = (this as? String).orEmpty()

dependencies {
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("com.google.android.gms:play-services-auth:21.5.0")
    implementation("io.noties:prism4j:2.0.0") {
        exclude(group = "org.jetbrains", module = "annotations-java5")
    }
    implementation("io.noties.markwon:core:4.6.2")
    implementation("io.noties.markwon:ext-tables:4.6.2")

    annotationProcessor("io.noties:prism4j-bundler:2.0.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")

    androidTestImplementation("androidx.test:core:1.6.1")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
}
