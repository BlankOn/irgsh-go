release:
	mkdir -p tmp
	# Temporarily backup the original files to inject version string
	cp -rf chief/main.go tmp/chief-main.go
	cp -rf builder/main.go tmp/builder-main.go
	cp -rf iso/main.go tmp/iso-main.go
	cp -rf repo/main.go tmp/repo-main.go
	cp -rf cli/main.go tmp/cli-main.go
	# Assign version
	cat tmp/chief-main.go | sed "s/IRGSH_GO_VERSION/$$(cat VERSION)/g" > chief/main.go
	cat tmp/builder-main.go | sed "s/IRGSH_GO_VERSION/$$(cat VERSION)/g" > builder/main.go
	cat tmp/iso-main.go | sed "s/IRGSH_GO_VERSION/$$(cat VERSION)/g" > iso/main.go
	cat tmp/repo-main.go | sed "s/IRGSH_GO_VERSION/$$(cat VERSION)/g" > repo/main.go
	cat tmp/cli-main.go | sed "s/IRGSH_GO_VERSION/$$(cat VERSION)/g" > cli/main.go
	# Build
	make build
	# Bundling
	mkdir -p irgsh-go/usr/bin
	mkdir -p irgsh-go/etc/irgsh
	mkdir -p irgsh-go/etc/init.d
	mkdir -p irgsh-go/usr/share/irgsh
	cp -rf bin/* irgsh-go/usr/bin/
	cp -rf utils/config.yml irgsh-go/etc/irgsh/
	cp -rf utils/init/* irgsh-go/etc/init.d/
	cp -rf -R utils/reprepro-template irgsh-go/usr/share/irgsh/reprepro-template
	tar -zcvf release.tar.gz irgsh-go
	mkdir -p target
	mv release.tar.gz target/
	# Clean up
	rm -rf irgsh-go
	cp -rf tmp/chief-main.go chief/main.go
	cp -rf tmp/builder-main.go builder/main.go
	cp -rf tmp/iso-main.go iso/main.go
	cp -rf tmp/repo-main.go repo/main.go
	cp -rf tmp/cli-main.go cli/main.go

release-in-docker: release
	# It's possible this release command will be used inside a container
	# Let it rewriteable for host environment
	chmod -vR a+rw target
	chown -vR :users target

preinstall:
	sudo /etc/init.d/irgsh-chief stop || true
	sudo /etc/init.d/irgsh-builder stop || true
	sudo /etc/init.d/irgsh-iso stop || true
	sudo /etc/init.d/irgsh-repo stop || true
	sudo killall irgsh-chief || true
	sudo killall irgsh-builder || true
	sudo killall irgsh-iso || true
	sudo killall irgsh-repo || true

build-in-docker:
	cp -rf utils/docker/build/Dockerfile .
	docker build --no-cache -t irgsh-build .
	docker run -v $(pwd)/target:/tmp/src/target irgsh-build make release-in-docker

build:
	mkdir -p bin
	cp -rf chief/utils.go builder/utils.go
	cp -rf chief/utils.go iso/utils.go
	cp -rf chief/utils.go repo/utils.go
	cp -rf chief/utils.go cli/utils.go
	go build -o ./bin/irgsh-chief ./chief
	go build -o ./bin/irgsh-builder ./builder
	go build -o ./bin/irgsh-iso ./iso
	go build -o ./bin/irgsh-repo ./repo
	go build -o ./bin/irgsh-cli ./cli
	rm builder/utils.go
	rm iso/utils.go
	rm repo/utils.go

build-install: preinstall build
	sudo cp -rf ./bin/irgsh-chief /usr/bin/irgsh-chief
	sudo cp -rf ./bin/irgsh-builder /usr/bin/irgsh-builder
	sudo cp -rf ./bin/irgsh-iso /usr/bin/irgsh-iso
	sudo cp -rf ./bin/irgsh-repo /usr/bin/irgsh-repo
	sudo cp -rf ./bin/irgsh-cli /usr/bin/irgsh-cli
	sudo cp -rf ./bin/irgsh-cli /usr/bin/irgsh-cli
	sudo /etc/init.d/irgsh-chief start
	sudo /etc/init.d/irgsh-builder start
	sudo /etc/init.d/irgsh-iso start
	sudo /etc/init.d/irgsh-repo start

test:
	mkdir -p tmp
	cp -rf chief/utils.go builder/utils.go
	cp -rf chief/utils.go iso/utils.go
	cp -rf chief/utils.go repo/utils.go
	go test -race -coverprofile=coverage.txt -covermode=atomic ./builder
	go test -race -coverprofile=coverage.txt -covermode=atomic ./iso
	go test -race -coverprofile=coverage.txt -covermode=atomic ./repo

coverage:test
	go tool cover -html=coverage.txt

irgsh-chief:
	go build -o ./bin/irgsh-chief ./chief && ./bin/irgsh-chief

irgsh-builder-init:
	go build -o ./bin/irgsh-builder ./builder && ./bin/irgsh-builder init

irgsh-builder:
	go build -o ./bin/irgsh-builder ./builder && ./bin/irgsh-builder

irgsh-iso:
	go build -o ./bin/irgsh-iso ./iso && ./bin/irgsh-iso

irgsh-repo-init:
	go build -o ./bin/irgsh-repo ./repo && ./bin/irgsh-repo init

irgsh-repo:
	go build -o ./bin/irgsh-repo ./repo && ./bin/irgsh-repo

redis:
	docker run -d --network host redis

submit:
	curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"https://github.com/BlankOn/bromo-theme.git","packageUrl":"https://github.com/BlankOn-packages/bromo-theme.git"}' http://localhost:8080/api/v1/submit
