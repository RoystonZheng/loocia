all: install

install: clean gen_output build
	mkdir -p output/bin
	mv aihot-server output/bin/

gen_output:
	mkdir -p output/log
	mkdir -p output/conf
	cp -rf conf/* output/conf/
	cp -rf control.sh output/

clean:
	rm -rf output/bin
	rm -rf output/conf
	rm -rf output/control.sh

build:
	go build -o aihot-server

run:
	./output/bin/aihot-server # 前台运行,方便退出

