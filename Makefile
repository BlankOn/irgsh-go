irgsh-chief:
	cd chief && go build && ./chief -c ../config.yml

irgsh-builder:
	cd builder && go build && ./builder -c ../config.yml

irgsh-builder-init:
	cd builder && go build && ./builder init

irgsh-repo:
	cd repo && go build && ./repo -c ../config.yml

redis:
	docker run -d --network host redis

submit:
	curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"git@github.com:BlankOn/bromo-theme.git","packageUrl":"git@github.com:blankon-packages/bromo-theme.git"}' http://localhost:8080/api/v1/submit
