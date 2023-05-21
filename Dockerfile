FROM golang:1.17.1-alpine
WORKDIR /app
COPY ./go.mod .
COPY ./go.sum .
RUN go mod download
COPY ./main.go ./
RUN go build
CMD ./transmission-bot