add_library(stack_buffer_overflow stack_buffer_overflow.cpp)
add_fuzz_test(stack_buffer_overflow_fuzztest stack_buffer_overflow_fuzztest.cpp)
target_link_libraries(stack_buffer_overflow_fuzztest PRIVATE stack_buffer_overflow)