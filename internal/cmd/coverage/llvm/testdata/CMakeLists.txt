cmake_minimum_required(VERSION 3.16)
project(cmake_example)

set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

enable_testing()

find_package(cifuzz NO_SYSTEM_ENVIRONMENT_PATH)
enable_fuzz_testing()

add_subdirectory(src)

add_fuzz_test(my_fuzz_test my_fuzz_test.cpp)
target_link_libraries(my_fuzz_test PRIVATE exploreMe)