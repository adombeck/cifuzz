add_library(heap_buffer_overflow heap_buffer_overflow.cpp)
add_fuzz_test(heap_buffer_overflow_fuzztest heap_buffer_overflow_fuzztest.cpp)
target_link_libraries(heap_buffer_overflow_fuzztest PRIVATE heap_buffer_overflow)
