module code-intelligence.com/cifuzz

go 1.20

require (
	atomicgo.dev/keyboard v0.2.9
	github.com/Masterminds/semver v1.5.0
	github.com/alessio/shellescape v1.4.1
	github.com/alexflint/go-filemutex v1.2.0
	github.com/docker/docker v24.0.4+incompatible
	github.com/gen2brain/beeep v0.0.0-20230602101333-f384c29b62dd
	github.com/gookit/color v1.5.3
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/hokaccha/go-prettyjson v0.0.0-20211117102719-0474bc63780f
	github.com/mattn/go-zglob v0.0.4
	github.com/mitchellh/ioprogress v0.0.0-20180201004757-6a23b12fa88e
	github.com/moby/term v0.5.0
	github.com/opencontainers/image-spec v1.1.0-rc4
	github.com/otiai10/copy v1.9.0
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/pkg/errors v0.9.1
	github.com/pterm/pterm v0.12.62
	github.com/spf13/cobra v1.7.0
	github.com/spf13/viper v1.16.0
	github.com/u-root/u-root v0.11.1-0.20230701062237-921c08deecd7
	golang.org/x/net v0.22.0
	golang.org/x/sync v0.3.0
	golang.org/x/term v0.18.0
)

// TODO: Revert when https://github.com/otiai10/copy/pull/94 is merged
replace github.com/otiai10/copy v1.9.0 => github.com/adombeck/copy v0.0.0-20221129184714-7b5f9872f143

require (
	atomicgo.dev/cursor v0.1.1 // indirect
	atomicgo.dev/schedule v0.0.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/containerd/console v1.0.4-0.20230313162750-1ae8d489ac81 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/go-toast/toast v0.0.0-20190211030409-01e6764cf0a4 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.3 // indirect
	gotest.tools/v3 v3.4.0 // indirect
)

require (
	github.com/fatih/color v1.15.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.0.8 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.4
	github.com/subosito/gotenv v1.4.2 // indirect
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1
	golang.org/x/sys v0.18.0
	golang.org/x/text v0.14.0
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
)
