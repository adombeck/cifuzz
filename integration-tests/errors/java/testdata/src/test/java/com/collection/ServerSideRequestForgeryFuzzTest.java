package com.collection;

import java.net.Socket;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class ServerSideRequestForgeryFuzzTest {
    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) throws Exception {
        String hostname = data.consumeString(15);
        try (Socket s = new Socket(hostname, 80)) {
            s.getInetAddress();
        }
    }
}