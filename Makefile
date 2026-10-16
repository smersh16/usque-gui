.PHONY: build test pkg-aarch64 pkg-mipsel pkg-mips pkg-all feed clean

build:
	go build -o out/usque-gui .

test:
	go vet ./... && go test ./...

pkg-aarch64:
	sh scripts/build-ipk.sh arm64 aarch64-3.10

pkg-mipsel:
	sh scripts/build-ipk.sh mipsle mipsel-3.4 softfloat

pkg-mips:
	sh scripts/build-ipk.sh mips mips-3.4 softfloat

pkg-all: pkg-aarch64 pkg-mipsel pkg-mips

feed: pkg-all
	sh scripts/mkfeed.sh

clean:
	rm -rf out
