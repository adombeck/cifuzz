CXXFLAGS += -std=c++11
# Disable optimizations to avoid that the compiler removes code which
# affects the stack trace (e.g. inlining of functions).
CXXFLAGS += -O0

clean:
	rm -rf build *.o *.a *.so.* *.so a.out

# This is to verify that we find the transitive dependencies with the ldd approach
build/libbug.so: src/bug/trigger_bugs.cpp src/bug/trigger_bugs.h
	mkdir -p build
	${CXX} ${CXXFLAGS} -shared -fpic -o build/libbug.so $<

build/libexplore.so.4.2: src/explore/explore_me.cpp src/explore/explore_me.h build/libbug.so
	mkdir -p build
	${CXX} ${CXXFLAGS} -shared -fpic -o build/libexplore.so.4.2 -Isrc -Lbuild -lbug $<

build/libexplore.so.4: build/libexplore.so.4.2
	mkdir -p build
	ln -s libexplore.so.4.2 build/libexplore.so.4

build/libexplore.so: build/libexplore.so.4
	mkdir -p build
	ln -s libexplore.so.4 build/libexplore.so

build/my_fuzz_test: build/libexplore.so
	@echo "Building $@"
	mkdir -p build
	cp -r my_fuzz_test_inputs build/
	cp my_fuzz_test.dict build/
	# The FUZZ_TEST_CXXFLAGS and FUZZ_TEST_LDFLAGS environment variables
	# are set by cifuzz when it executes the build command. Those must
	# be passed to the compiler and linker (compiling and linking is
	# done in a single invocation here, so we pass both to $CXX here).
	# We dont set the rpath here so to verify that cifuzz run correctly configure library paths
	${CXX} ${CXXFLAGS} ${FUZZ_TEST_CXXFLAGS} ${FUZZ_TEST_LDFLAGS} -o build/my_fuzz_test my_fuzz_test.cpp -Lbuild -lexplore

.PHONY: my_fuzz_test
my_fuzz_test: build/my_fuzz_test

crashing_fuzz_test.o: src/coverage/crashing_fuzz_test.c
	${CC} ${CFLAGS} ${FUZZ_TEST_CFLAGS} -c $<

crashing_fuzz_test: crashing_fuzz_test.o
	${CC} -o $@ crashing_fuzz_test.o ${LDFLAGS} ${FUZZ_TEST_LDFLAGS}
