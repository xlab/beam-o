all:

build:
	GodyBuilder build --image astranet/gody_build_ubuntu --pkg github.com/xlab/beam-o/cmd/beam-o-client --out bin/
	GodyBuilder build --image astranet/gody_build_ubuntu --pkg github.com/xlab/beam-o/cmd/beam-o-server --out bin/
