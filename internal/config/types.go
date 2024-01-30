package config

type FuzzTestType string

const (
	CPP        FuzzTestType = "cpp"
	Java       FuzzTestType = "java"
	Kotlin     FuzzTestType = "kotlin"
	JavaScript FuzzTestType = "js"
	TypeScript FuzzTestType = "ts"
)

// map of supported test types -> label:value
var SupportedTestTypes = map[string]string{
	"C/C++":      string(CPP),
	"Java":       string(Java),
	"Kotlin":     string(Kotlin),
	"JavaScript": string(JavaScript),
	"TypeScript": string(TypeScript),
}

type GradleBuildLanguage string

const (
	GradleGroovy GradleBuildLanguage = "groovy"
	GradleKotlin GradleBuildLanguage = "kotlin"
)

type Engine string

const (
	Libfuzzer Engine = "libfuzzer"
)
