FROM golang:1.24 AS build
WORKDIR /src
COPY services/api/go.mod services/api/go.sum ./
RUN go mod download
COPY services/api ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
ENTRYPOINT ["/api"]
