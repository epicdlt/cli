.PHONY: dep build docker release install test backup

build: 
	go build -o epic

install: build
	sudo cp epic /usr/local/bin/epic

docker: 
	docker build -t treeder/epic:latest .

run: build
	./epic start

test:
	go test ./...

bench:
	go test ./... -run=XXX -bench=.
	