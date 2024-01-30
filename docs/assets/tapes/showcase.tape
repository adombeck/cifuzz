Output docs/assets/showcase.gif

Require cifuzz
Require gradle
Require nvim

Set Theme "TokyoNight"
Set Framerate 30
Set FontSize 16
Set Width 1800
Set Height 900

Sleep 100ms

Hide
Type "cd examples/gradle"
Enter
Show

Sleep 1s

Type "nvim build.gradle"
Enter
Sleep 1s

Type "10jo"
Type@20ms "id 'com.code-intelligence.cifuzz' version '1.5.0'"
Escape
Sleep 3
Type ":wq"
Enter

Type@30ms "cifuzz create -o src/test/java/com/example/MyFuzzTest.java"
Enter
Sleep 1s
Down
Sleep 500ms
Enter
Sleep 3s

Type@30ms "nvim src/test/java/com/example/MyFuzzTest.java"
Enter
Sleep 500ms

Type "8j"
Sleep 1s
Type "ci}"
Type@20ms "int a = data.consumeInt();"
Enter
Type@20ms "int b = data.consumeInt();"
Enter
Type@20ms "String c = data.consumeRemainingAsString();"
Enter
Enter
Type@20ms "ExploreMe ex = new ExploreMe(a);"
Enter
Type@20ms "ex.exploreMe(b, c);"

Escape
Sleep 3s

Type ":wq"
Enter

Type "cifuzz run com.example.MyFuzzTest::myFuzzTest"
Enter

Sleep 6s

Type "cifuzz findings"
Enter

Sleep 4s
