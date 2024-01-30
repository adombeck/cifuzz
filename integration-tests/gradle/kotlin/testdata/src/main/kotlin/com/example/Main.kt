package com.example

fun main(args: Array<String>) {
    println("Hello World!")
    val ex = ExploreMe(1)
    ex.exploreMe(1, "A")
    ex.exploreMe(2147483647, "A")
    ex.exploreMe(2000000123, "A")
    ex.exploreMe(2000000123, "FUZZING")
    // Try adding program arguments via Run/Debug configuration.
    // Learn more about running applications: https://www.jetbrains.com/help/idea/running-applications.html.
    // println("Program arguments: ${args.joinToString()}")
}