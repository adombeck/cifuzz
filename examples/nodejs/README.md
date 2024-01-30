# cifuzz nodejs javascript example

This is a simple nodejs/npm based project, already configured with
**cifuzz**. It should quickly produce a finding, but slow enough to
see the progress of the fuzzer.

To start make sure you installed **cifuzz** according to the
main [README](../../README.md).

After you ran `npm install`, you can start fuzzing with

```bash
cifuzz run FuzzTestCase
```

## Coverage

cifuzz can generate HTML coverage reports by running:

```bash
cifuzz coverage FuzzTestCase
```
