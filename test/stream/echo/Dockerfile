# syntax=docker/dockerfile:1
# Tiny static TCP/UDP echo server for the stream-host test harness.
FROM golang:1.27.1-alpine AS build
ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /src
COPY go.mod main.go ./
RUN go build -trimpath -ldflags "-s -w" -o /out/echo .

FROM scratch
COPY --from=build /out/echo /echo
EXPOSE 9000
ENTRYPOINT ["/echo"]
