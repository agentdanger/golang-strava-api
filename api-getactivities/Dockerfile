FROM golang:1.20
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go build -o main .
RUN go mod download
EXPOSE 8080
CMD ["/app/main"]