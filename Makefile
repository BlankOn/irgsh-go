irgsh-chief:
	cd chief && go build && ./chief -c ../config.yml

irgsh-builder:
	cd builder && go build && ./builder -c ../config.yml

irgsh-repo:
	cd repo && go build && ./repo -c ../config.yml

redis:
	docker run -d --network host redis
