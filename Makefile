build:
	docker build -t golacas .
run:
	docker run -ti -p 8080:8080 --rm golacas /go/bin/golacas
