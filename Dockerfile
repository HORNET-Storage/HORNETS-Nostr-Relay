FROM golang:20.7

WORKDIR /go/src/app
COPY ./ .

RUN go get -d -v ./...
RUN go install -v ./...
RUN make
