# Note: Keep the clang flags used below in sync with internal/cmd/run/run.go
#       Explanations of these flags are provided in that file.
function(enable_fuzz_testing)
  if(${CMAKE_MINIMUM_REQUIRED_VERSION} VERSION_LESS 3.16)
    message(FATAL_ERROR "cifuzz: cmake_minimum_required(VERSION 3.16) or higher required, but the current project declares a minimum version of ${CMAKE_MINIMUM_REQUIRED_VERSION}")
  endif()

  # Remove the metadata directory we populate with fuzz test target information at configuration time so that e.g.
  # metadata for renamed or removed targets doesn't linger around.
  file(REMOVE_RECURSE "${CMAKE_BINARY_DIR}/$<CONFIG>/.cifuzz")

  # Conceptually, "building for fuzzing" is similar to a build type such as Release or RelWithDebInfo. We instead use
  # a cache variable that adds flags to a base configuration we assume to be RelWithDebInfo for multiple reasons:
  # 1. Custom build types require defining a potentially unknown set of cache variables and are thus hard to maintain.
  # 2. Since custom build types store the flags in cache variables, cifuzz updates changing the flags would require
  #    regenerating CMake build directories rather than just building them.
  # 3. Many projects contain checks for the name of the build type, which makes us more compatible if we use an existing
  #    one.
  if(CIFUZZ_TESTING)
    add_compile_definitions(FUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION)
    if(WIN32)
      add_compile_options(
          # Allow the compiler to inline more aggressively. This overrides the (questionable?) default of /Ob1 set by
          # CMake's RelWithDebInfo configuration (see https://stackoverflow.com/a/66089368/297261). Given that it also
          # sets /Zi, which implies /Zo, which promises that it "tells the compiler to generate additional debugging
          # information for local variables and inlined functions" (see
          # https://docs.microsoft.com/en-us/cpp/build/reference/zo-enhance-optimized-debugging?view=msvc-170).
          /Ob2
          # clang-cl's equivalent of -fno-omit-frame-pointer.
          /Oy-
          # Undefine NDEBUG, which is explicitly defined by the RelWithDebInfo CMake configuration, so that asserts are
          # kept.
          /UNDEBUG
          # Link the CRT statically so that ASan is also linked statically.
          # TODO(fmeum): Remove once ASan has been stabilized and clang_rt.asan_dynamic-x86_64.dll is available in the
          #  default PATH, e.g. in System32.
          # https://stackoverflow.com/a/66532115/297261
          /MT
          # See https://github.com/llvm/llvm-project/issues/56300
          -D_DISABLE_VECTOR_ANNOTATION
          -D_DISABLE_STRING_ANNOTATION
      )

      # Split the version string by periods into a list
      string(REPLACE "." ";" version_list ${CMAKE_CXX_COMPILER_VERSION})
      # Get the first element of the list, which is the major version
      list(GET version_list 0 CXX_COMPILER_MAJOR_VERSION)
      # Add Visual Studio directory for clang runtime libraries to link directories 
      link_directories("$ENV{VSINSTALLDIR}\\VC\\Tools\\Llvm\\x64\\lib\\clang\\${CXX_COMPILER_MAJOR_VERSION}\\lib\\windows")
      add_link_options(
          # /INCREMENTAL is enabled by default with RelWithDebInfo, but is unsupported with ASan and potentially impacts
          # performance by padding functions.
          # https://gitlab.kitware.com/cmake/cmake/-/issues/20812
          /INCREMENTAL:NO
      )
    else()
      add_compile_options(
          -fno-omit-frame-pointer
          # Undefine NDEBUG, which is explicitly defined by the RelWithDebInfo CMake configuration, so that asserts are
          # kept.
          -UNDEBUG
          # disable optimizations to ensure high debuggability
          -O0
      )
    endif()
  endif()

  if(CIFUZZ_ENGINE STREQUAL libfuzzer)
    # We also use the libfuzzer engine in coverage mode, but don't want fuzzing instrumentation to be applied in that
    # case.
    if(NOT coverage IN_LIST CIFUZZ_SANITIZERS)
      add_compile_options(-fsanitize=fuzzer)
      if(WIN32)
          # On Windows the option "-fsanitize=fuzzer" doesn't take care of linking the runtime libraries
        add_link_options("clang_rt.fuzzer-x86_64.lib")
      endif()
    endif()
  endif()

  foreach(sanitizer IN LISTS CIFUZZ_SANITIZERS)
    if(sanitizer STREQUAL address)
      add_compile_options(
          -fsanitize=address
          -fsanitize-recover=address
          -fsanitize-address-use-after-scope
          # Disable source fortification, which is currently not supported
          # in combination with ASan, see https://github.com/google/sanitizers/issues/247
          # Note that this does not override a user-specified -D_FORTIFY_SOURCE,
          # because the flags we add here come before user-specified flags.
          # It's still useful to disable source fortification which the
          # toolchain or distribution might have enabled by default.
          -U_FORTIFY_SOURCE
      )
      if(WIN32)
        add_link_options(
          "clang_rt.asan-preinit-x86_64.lib"
          "clang_rt.asan-x86_64.lib"
          "clang_rt.asan_cxx-x86_64.lib"
        )
      else()
        add_link_options(-fsanitize=address)
      endif()
    elseif(sanitizer STREQUAL undefined)
      add_compile_options(-fsanitize=undefined)
      if(NOT WIN32)
        add_link_options(-fsanitize=undefined)
      endif()
    elseif(sanitizer STREQUAL coverage)
      add_compile_options(
          -fprofile-instr-generate
          -fcoverage-mapping
          # Disable source fortification to ensure that coverage builds
          # reach all code reached by ASan builds.
          -U_FORTIFY_SOURCE
      )
      if(NOT APPLE)
        # LLVM's continuous coverage mode currently requires compile-time support on non-macOS platforms. This is only
        # really working as of clang 14 though, earlier versions are affected by runtime crashes.
        if (((NOT DEFINED CMAKE_C_COMPILER_VERSION) OR ("${CMAKE_C_COMPILER_VERSION}" VERSION_GREATER_EQUAL 14)) AND
          ((NOT DEFINED CMAKE_CXX_COMPILER_VERSION) OR ("${CMAKE_CXX_COMPILER_VERSION}" VERSION_GREATER_EQUAL 14)))
          add_compile_options(-mllvm -runtime-counter-relocation)
        endif()
      endif()
      add_link_options(-fprofile-instr-generate)
      if(WIN32)
        add_link_options(
          "clang_rt.fuzzer-x86_64.lib"
          "clang_rt.profile-x86_64.lib"
        )
      endif()
    elseif(sanitizer STREQUAL gcov)
      if(WIN32)
        message(FATAL_ERROR "cifuzz: coverage builds are not yet supported")
      else()
        # We useand gcov style coverage instrumentation instead of llvm-cov since CLion does not correctly collect
        # coverage for shared libraries with llvm-cov. The flag is supported by both gcc and clang.
        # TODO: Investigate whether this extra sanitizer is needed once the following issue has been fixed:
        #  https://youtrack.jetbrains.com/issue/CPP-29628/LLVM-Code-coverage-not-usable-with-shared-libraries
        add_compile_options(--coverage)
        add_link_options(--coverage)
      endif()
    else()
      message(FATAL_ERROR "cifuzz: Unsupported value in CIFUZZ_SANITIZERS: ${sanitizer}")
    endif()
  endforeach()
endfunction()

function(add_fuzz_test name)
  set(_options)
  set(_one_value_args)
  set(_multi_value_args DEPENDENCIES INCLUDE_DIRS SOURCES )
  cmake_parse_arguments(PARSE_ARGV 1 _args "${_options}" "${_one_value_args}" "${_multi_value_args}")

  if( NOT _args_SOURCES )
    set(_args_sources ${_args_UNPARSED_ARGUMENTS})
  else()
    set(_args_sources ${_args_SOURCES})
  endif()

  add_executable("${name}" ${_args_sources})

  if(CIFUZZ_USE_DEPRECATED_MACROS)
    # The old fuzz macro header is injected via the compile command line. It does not live under the include directory
    # so that is not offered to fuzz tests using the new macros via include path IDE completions.
    set(_fuzz_macro_header "$<SHELL_PATH:${CIFUZZ_INCLUDE_DIR}/legacy/fuzz_macro.h>")
    if(WIN32)
      target_compile_options("${name}" PRIVATE /FI"${_fuzz_macro_header}")
    else()
      target_compile_options("${name}" PRIVATE "-include${_fuzz_macro_header}")
    endif()
  endif()
  
  list( APPEND _args_INCLUDE_DIRS SYSTEM PRIVATE "${CIFUZZ_INCLUDE_DIR}" )
  target_include_directories("${name}" ${_args_INCLUDE_DIRS} )

  if( _args_DEPENDENCIES )
    target_link_libraries( "${name}" ${_args_DEPENDENCIES} )
  endif()
  
  # This macro is consumed by cifuzz.h and cifuzz_launcher.c.
  target_compile_definitions("${name}" PRIVATE CIFUZZ_TEST_NAME="${name}")

  get_property(_enabled_languages GLOBAL PROPERTY ENABLED_LANGUAGES)

  if(CIFUZZ_ENGINE STREQUAL replayer)
    # The replayer is written so that it can be compiled as both C and C++.
    # Since we do not have control over the enabled languages, we add the
    # replayer with a source file extension matching the enabled language.
    if(C IN_LIST _enabled_languages)
      set(_replayer_src "${CIFUZZ_REPLAYER_C_SRC}")
    else()
      if (NOT CXX IN_LIST _enabled_languages)
        message(FATAL "cifuzz: At least one of C and CXX has to be an enabled language")
      endif()
      set(_replayer_src "${CIFUZZ_REPLAYER_CXX_SRC}")
    endif()
    target_sources("${name}" PRIVATE "${_replayer_src}")
    if(coverage IN_LIST CIFUZZ_SANITIZERS)
      # Never instrument the replayer file for coverage.
      set_source_files_properties("${_replayer_src}"
                                  PROPERTIES COMPILE_FLAGS
                                  "-fno-profile-instr-generate -fno-coverage-mapping")
    elseif(gcov IN_LIST CIFUZZ_SANITIZERS)
      # Never instrument the replayer file for coverage.
      set_source_files_properties("${_replayer_src}"
                                  PROPERTIES COMPILE_FLAGS
                                  "-fprofile-exclude-files=.*")
    elseif(CIFUZZ_SANITIZERS)
      target_compile_definitions("${name}" PRIVATE CIFUZZ_HAS_SANITIZER)
    endif()
  elseif(CIFUZZ_ENGINE STREQUAL libfuzzer)
    if(CMAKE_CXX_COMPILER_ID STREQUAL "Clang" OR ((NOT "CXX" IN_LIST _enabled_languages) AND (CMAKE_C_COMPILER_ID STREQUAL "Clang")))
      if(NOT WIN32)
        target_link_options("${name}" PRIVATE -fsanitize=fuzzer)
      endif()
    else()
      set(_clang_description "clang/clang++")
      if (CMAKE_CXX_COMPILER_ID STREQUAL "AppleClang")
        set(_clang_description "(non-Apple) clang/clang++")
      endif()
      message(FATAL_ERROR "cifuzz: ${CMAKE_CXX_COMPILER_ID} compiler is not supported with the libfuzzer engine.\n"
        "Either specify the full path to ${_clang_description} in CC/CXX or ensure that it is listed before other compilers in your PATH.\n"
        "After that remove ${CMAKE_BINARY_DIR} and try again.")
    endif()
    # The launcher is written so that it can be compiled as both C and C++.
    # Since we do not have control over the enabled languages, we add the
    # launcher with a source file extension matching the enabled language.
    if(C IN_LIST _enabled_languages)
      set(_launcher_src "${CIFUZZ_LAUNCHER_C_SRC}")
    else()
      if (NOT CXX IN_LIST _enabled_languages)
        message(FATAL "CIFuzz: At least one of C and CXX has to be an enabled language")
      endif()
      set(_launcher_src "${CIFUZZ_LAUNCHER_CXX_SRC}")
    endif()
    if (coverage IN_LIST CIFUZZ_SANITIZERS)
      # Never instrument the launcher file for coverage.
      set_source_files_properties("${_launcher_src}"
                                  PROPERTIES COMPILE_FLAGS
                                  "-fno-profile-instr-generate -fno-coverage-mapping")
    endif()
    target_sources("${name}" PRIVATE "${_launcher_src}")
    if((address IN_LIST CIFUZZ_SANITIZERS) OR (undefined IN_LIST CIFUZZ_SANITIZERS))
      # The macOS linker doesn't support --wrap, so we fall back to a different strategy that doesn't require any linker
      # flags.
      # See src/dumper.c for details.
      if(NOT APPLE)
        if(WIN32)
          # ALTERNATENAME replaces the function call to _ZN11__sanitizer6PrintfEPKcz with ?Printf@fuzzer@@YAXPEBDZZ.
          # _ZN11__sanitizer6PrintfEPKcz is the gcc-style mangled name of __sanitizer:Printf(const char *format, ...).
          # ?Printf@fuzzer@@YAXPEBDZZ is the mangled name of fuzzer::Printf(const char *format, ...) on Windows,
          # which is contained in clang_rt.fuzzer-x86_64.lib.
          # We rely on that redirection, because ?Printf@fuzzer@@YAXPEBDZZ cannot be called directly in dumper.c.
          target_link_options("${name}" PRIVATE
            -wrap:__sanitizer_set_death_callback
            /ALTERNATENAME:_ZN11__sanitizer6PrintfEPKcz=?Printf@fuzzer@@YAXPEBDZZ
          )
        else()
          target_link_options("${name}" PRIVATE -Wl,--wrap=__sanitizer_set_death_callback)
        endif()
      endif()
      if(C IN_LIST _enabled_languages)
        set(_dumper_src "${CIFUZZ_DUMPER_C_SRC}")
      else()
        if (NOT CXX IN_LIST _enabled_languages)
          message(FATAL "CIFuzz: At least one of C and CXX has to be an enabled language")
        endif()
        set(_dumper_src "${CIFUZZ_DUMPER_CXX_SRC}")
      endif()
      target_sources("${name}" PRIVATE "${_dumper_src}")
    endif()
  else()
    message(FATAL_ERROR "cifuzz: Unsupported value for CIFUZZ_ENGINE: ${CIFUZZ_ENGINE}")
  endif()

  # On macOS, debug information is only contained in the object files by default. llvm-symbolizer, which we use to
  # resolve addresses in stack traces to source location, doesn't read the object files. We thus need to invoked
  # dsymutil to link the debug information into <name>.dSYM explicitly.
  if(APPLE)
    add_custom_command(TARGET "${name}"
                       POST_BUILD
                       COMMAND dsymutil ARGS $<TARGET_FILE:${name}>
                       BYPRODUCTS "${name}.dSYM")
  endif()

  set(_seed_corpus_suffix _inputs)
  set(_source_seed_corpus "${CMAKE_CURRENT_SOURCE_DIR}/${name}${_seed_corpus_suffix}")
  # Convert path separators to '\' (Windows only) and escape all backslashes for a C string literal.
  # In the regex strings below, one level of escaping is for the CMake string and another one to get a literal backslash
  # in a regex.
  if(WIN32)
    string(REGEX REPLACE "/" "\\\\" _source_seed_corpus "${_source_seed_corpus}")
  endif()
  string(REGEX REPLACE "\\\\" "\\\\\\\\" _source_seed_corpus "${_source_seed_corpus}")
  # Compile the path to the seed corpus, which lives under the source root, into the fuzz test binary as it is built
  # out-of-tree. An alternative could be to symlink the seed corpus to a well-known location next to the binary, but
  # symlinks are not always available on Windows (junctions exist, but may cause issues with tools that are unaware of
  # them and are not easy to deal with using just POSIX functions).
  target_compile_definitions("${name}" PRIVATE CIFUZZ_SEED_CORPUS="${_source_seed_corpus}")

  # Coverage builds should always run over the full generated corpus in addition to the seed corpus.
  if(coverage IN_LIST CIFUZZ_SANITIZERS OR gcov IN_LIST CIFUZZ_SANITIZERS)
    set(_source_generated_corpus "${CMAKE_SOURCE_DIR}/.cifuzz-corpus/${name}")
    if(WIN32)
      string(REGEX REPLACE "/" "\\\\" _source_generated_corpus "${_source_generated_corpus}")
    endif()
    string(REGEX REPLACE "\\\\" "\\\\\\\\" _source_generated_corpus "${_source_generated_corpus}")
    target_compile_definitions("${name}" PRIVATE CIFUZZ_GENERATED_CORPUS="${_source_generated_corpus}")
  endif()

  set(_dict_suffix .dict)
  set(_source_dict "${CMAKE_CURRENT_SOURCE_DIR}/${name}${_dict_suffix}")
  # Convert path separators to '\' (Windows only) and escape all backslashes for a C string literal.
  # In the regex strings below, one level of escaping is for the CMake string and another one to get a literal backslash
  # in a regex.
  if(WIN32)
    string(REGEX REPLACE "/" "\\\\" _source_dict "${_source_dict}")
  endif()
  string(REGEX REPLACE "\\\\" "\\\\\\\\" _source_dict "${_source_dict}")

  # Collect a mapping from CMake target names to information required by cifuzz. Currently, this includes the path of
  # the fuzz test executable as well as of its seed corpus and its default dictionary.
  # We don't use add_custom_command here as we want the mapping to exist already after the configure step, not only
  # after the build step - this way, it is comparatively cheap to update the mapping since the actual build tool doesn't
  # have to run. IDEs may even refresh the metadata automatically for us.
  # Note: Removed and renamed targets leave behind their entry in this mapping. Since these files are cheap to
  #       regenerate, cifuzz can just delete the entire .cifuzz directory before each build (see enable_fuzz_testing).
  set(_executable_info_file "${CMAKE_BINARY_DIR}/$<CONFIG>/.cifuzz/fuzz_tests/${name}/executable")
  file(GENERATE
       OUTPUT "$<SHELL_PATH:${_executable_info_file}>"
       CONTENT $<TARGET_FILE:${name}>)
  set(_seed_corpus_info_file "${CMAKE_BINARY_DIR}/$<CONFIG>/.cifuzz/fuzz_tests/${name}/seed_corpus")
  file(GENERATE
       OUTPUT "$<SHELL_PATH:${_seed_corpus_info_file}>"
       CONTENT "${_source_seed_corpus}")
  set(_dict_info_file "${CMAKE_BINARY_DIR}/$<CONFIG>/.cifuzz/fuzz_tests/${name}/dict")
  file(GENERATE
       OUTPUT "$<SHELL_PATH:${_dict_info_file}>"
       CONTENT "${_source_dict}")

  set(_test_name "${name}_regression_test")
  add_test(NAME "${_test_name}" COMMAND "${name}")
  set_tests_properties("${_test_name}" PROPERTIES LABELS "cifuzz_regression_test")

  # Define an install component cifuzz_internal_deps_${name} that, when "installed", prints the full paths of the
  # transitive runtime dependencies, including system libraries, of the fuzz target to stdout in the form:
  #
  # -- CIFUZZ RESOLVED /lib/x86_64-linux-gnu/libgcc_s.so.1
  # -- CIFUZZ RESOLVED /home/user/git/cifuzz/tools/cmake/testdata/build/src/utils/libhelper.so
  # -- CIFUZZ RESOLVED /lib/x86_64-linux-gnu/libstdc++.so.6
  #
  # If any library couldn't be resolved (unambiguously), it is reported with a leading UNRESOLVED or CONFLICTING.
  install(CODE "
    file(GET_RUNTIME_DEPENDENCIES
        RESOLVED_DEPENDENCIES_VAR _resolved_deps
        UNRESOLVED_DEPENDENCIES_VAR _unresolved_deps
        CONFLICTING_DEPENDENCIES_PREFIX _conflicting_deps
        EXECUTABLES \"$<TARGET_FILE:${name}>\"
    )

    foreach(_resolved_dep IN LISTS _resolved_deps)
        message(STATUS \"CIFUZZ RESOLVED \${_resolved_dep}\")
    endforeach()
    foreach(_unresolved_dep IN LISTS _unresolved_deps)
        message(STATUS \"CIFUZZ UNRESOLVED \${_unresolved_dep}\")
    endforeach()
    foreach(_conflicting_dep IN LISTS _conflicting_deps)
        message(STATUS \"CIFUZZ CONFLICTING \${_conflicting_dep}\")
    endforeach()
  " COMPONENT "cifuzz_internal_deps_${name}")
endfunction()
