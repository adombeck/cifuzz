package com.example;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class FuzzTestCase1 {
    @FuzzTest
    void myFuzzTest(FuzzedDataProvider data) {}
}
