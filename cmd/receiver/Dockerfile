FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/receiver ./cmd/receiver
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/receiver ./cmd/receiver

FROM scratch
COPY --from=build /out/receiver /receiver
EXPOSE 8081
ENTRYPOINT ["/receiver"]
