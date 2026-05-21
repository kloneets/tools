cmake_minimum_required(VERSION 3.22.1)

project(koko_spell)

add_library(
    koko_spell
    SHARED
    HunspellBridge.cpp
    hunspell/affentry.cxx
    hunspell/affixmgr.cxx
    hunspell/csutil.cxx
    hunspell/filemgr.cxx
    hunspell/hashmgr.cxx
    hunspell/hunspell.cxx
    hunspell/hunzip.cxx
    hunspell/phonet.cxx
    hunspell/replist.cxx
    hunspell/suggestmgr.cxx
)

target_include_directories(koko_spell PRIVATE hunspell)

target_compile_features(koko_spell PRIVATE cxx_std_17)
target_compile_options(koko_spell PRIVATE -fexceptions -frtti)

find_library(log-lib log)
target_link_libraries(koko_spell ${log-lib})
