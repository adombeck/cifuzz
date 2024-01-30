package com.example;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class FuzzTestCase2 {
    @FuzzTest
    void oneFuzzTest(FuzzedDataProvider data) {}

    @FuzzTest
    void anotherFuzzTest(FuzzedDataProvider data) {}
}
