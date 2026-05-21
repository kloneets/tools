#include <jni.h>

#include <memory>
#include <string>
#include <vector>

#include "hunspell.hxx"

namespace {

Hunspell* engineFromHandle(jlong handle) {
    return reinterpret_cast<Hunspell*>(handle);
}

std::string jstringToString(JNIEnv* env, jstring value) {
    if (value == nullptr) {
        return "";
    }
    const char* chars = env->GetStringUTFChars(value, nullptr);
    if (chars == nullptr) {
        return "";
    }
    std::string result(chars);
    env->ReleaseStringUTFChars(value, chars);
    return result;
}

}  // namespace

extern "C" JNIEXPORT jlong JNICALL
Java_com_kloneets_kokotools_HunspellNative_open(
    JNIEnv* env,
    jobject,
    jstring affPath,
    jstring dicPath
) {
    const std::string aff = jstringToString(env, affPath);
    const std::string dic = jstringToString(env, dicPath);
    try {
        return reinterpret_cast<jlong>(new Hunspell(aff.c_str(), dic.c_str()));
    } catch (...) {
        return 0L;
    }
}

extern "C" JNIEXPORT jboolean JNICALL
Java_com_kloneets_kokotools_HunspellNative_spell(
    JNIEnv* env,
    jobject,
    jlong handle,
    jstring word
) {
    Hunspell* engine = engineFromHandle(handle);
    if (engine == nullptr) {
        return JNI_FALSE;
    }
    return engine->spell(jstringToString(env, word)) ? JNI_TRUE : JNI_FALSE;
}

extern "C" JNIEXPORT jobjectArray JNICALL
Java_com_kloneets_kokotools_HunspellNative_suggest(
    JNIEnv* env,
    jobject,
    jlong handle,
    jstring word
) {
    Hunspell* engine = engineFromHandle(handle);
    const jclass stringClass = env->FindClass("java/lang/String");
    if (engine == nullptr || stringClass == nullptr) {
        return env->NewObjectArray(0, stringClass, nullptr);
    }

    const std::vector<std::string> suggestions = engine->suggest(jstringToString(env, word));
    jobjectArray result = env->NewObjectArray(
        static_cast<jsize>(suggestions.size()),
        stringClass,
        nullptr
    );
    for (jsize index = 0; index < static_cast<jsize>(suggestions.size()); ++index) {
        jstring suggestion = env->NewStringUTF(suggestions[static_cast<size_t>(index)].c_str());
        env->SetObjectArrayElement(result, index, suggestion);
        env->DeleteLocalRef(suggestion);
    }
    return result;
}

extern "C" JNIEXPORT void JNICALL
Java_com_kloneets_kokotools_HunspellNative_close(
    JNIEnv*,
    jobject,
    jlong handle
) {
    delete engineFromHandle(handle);
}
