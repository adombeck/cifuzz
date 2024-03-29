add_library(secrets secrets.cpp)
target_sources(secrets PUBLIC
               secrets.h)
set_property(TARGET secrets PROPERTY POSITION_INDEPENDENT_CODE ON)

if(WIN32)
  # libfuzzer has to be linked statically for clang-cl to get fuzzer
  # instrumentation, but that causes the linker to link in libFuzzer, even into a shared library. This seems to be the
  # cause of linker errors, so we fall back to linking statically here until the situation improves.
  # TODO: We may be able to craft our own fuzzer-no-link option by setting an appropriate /NODEFAULTLIB as a linker
  # flag on shared libraries only.
  add_library(parser parser.cpp)
else()
  add_library(parser SHARED parser.cpp)
  set_target_properties(parser PROPERTIES
                        VERSION 4.2
                        SOVERSION 4)
endif()
target_sources(parser PUBLIC
               parser.h)
target_include_directories(parser PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(parser PRIVATE secrets)
