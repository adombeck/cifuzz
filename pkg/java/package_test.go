package java

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPackageFromSource(t *testing.T) {
	testCases := []struct {
		sourceCode           io.Reader
		expecptedPackageName string
	}{
		{
			sourceCode: strings.NewReader(`
package com.example;
  `),
			expecptedPackageName: "com.example",
		},
		{
			sourceCode: strings.NewReader(`
// Test comment
package com.example;
  `),
			expecptedPackageName: "com.example",
		},
		{
			sourceCode: strings.NewReader(`
/*
Test block comment
*/
package com.example;
  `),
			expecptedPackageName: "com.example",
		},
		{
			sourceCode:           strings.NewReader(""),
			expecptedPackageName: "",
		},
		{
			sourceCode: strings.NewReader(`
invalidpackage com.example;
  `),
			expecptedPackageName: "",
		},
		{
			sourceCode: strings.NewReader(`
public class ExploreMe {
			`),
			expecptedPackageName: "",
		},
	}

	for _, testCase := range testCases {
		packageName := GetPackageFromSource(testCase.sourceCode)
		assert.Equal(t, testCase.expecptedPackageName, packageName)
	}
}
