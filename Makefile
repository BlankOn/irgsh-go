LDFLAGS := "-X main.version=$$(cat ./VERSION)"

release:
	# Build
	make build
	# Bundling
	mkdir -p irgsh-go/usr/bin
	mkdir -p irgsh-go/etc/irgsh
	mkdir -p irgsh-go/etc/init.d
	mkdir -p irgsh-go/lib/systemd/system
	mkdir -p irgsh-go/usr/share/irgsh
	cp -rf bin/* irgsh-go/usr/bin/
	cp -rf utils/config.yml irgsh-go/etc/irgsh/
	cp -rf utils/config.yml irgsh-go/usr/share/irgsh/config.yml
	cp -rf utils/init/* irgsh-go/etc/init.d/
	cp -rf utils/systemctl/* irgsh-go/lib/systemd/system
	cp -rf utils/scripts/init.sh irgsh-go/usr/share/irgsh/init.sh
	cp -rf -R utils/reprepro-template irgsh-go/usr/share/irgsh/reprepro-template
	tar -zcvf release.tar.gz irgsh-go
	mkdir -p target
	mv release.tar.gz target/

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
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-repo ./cmd/repo
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-chief ./cmd/chief
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-builder ./cmd/builder
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-iso ./cmd/iso
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-cli ./cmd/cli

build-install: release
	./install.sh
	sudo systemctl daemon-reload
	sudo /lib/systemd/systemd-sysv-install enable irgsh-chief
	sudo /lib/systemd/systemd-sysv-install enable irgsh-builder
	sudo /lib/systemd/systemd-sysv-install enable irgsh-repo
	sudo systemctl start irgsh-chief
	sudo systemctl start irgsh-builder
	sudo systemctl start irgsh-repo

test:
	mkdir -p tmp
	go test -race -coverprofile=coverage.txt -covermode=atomic ./cmd/builder
	go test -race -coverprofile=coverage.txt -covermode=atomic ./cmd/iso
	go test -race -coverprofile=coverage.txt -covermode=atomic ./cmd/repo

coverage:test
	go tool cover -html=coverage.txt

client:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-cli ./cmd/cli

chief:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-chief ./cmd/chief && DEV=1 ./bin/irgsh-chief

builder-init:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-builder ./cmd/builder && sudo DEV=1 ./bin/irgsh-builder init-base
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-builder ./cmd/builder && DEV=1 ./bin/irgsh-builder init-builder

builder:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-builder ./cmd/builder && DEV=1 ./bin/irgsh-builder

iso:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-iso ./cmd/iso && DEV=1 ./bin/irgsh-iso

repo-init:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-repo ./cmd/repo && DEV=1 ./bin/irgsh-repo init

repo:
	go build -ldflags $(LDFLAGS) -o ./bin/irgsh-repo ./cmd/repo && DEV=1 ./bin/irgsh-repo

redis:
	docker run -d --network host redis

submit:
	curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"https://github.com/BlankOn/bromo-theme.git","packageUrl":"https://github.com/BlankOn-packages/bromo-theme.git"}' http://localhost:8080/api/v1/submit
