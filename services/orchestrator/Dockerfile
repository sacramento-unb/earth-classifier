FROM golang:1.24 AS build
WORKDIR /src
COPY services/orchestrator/go.mod services/orchestrator/go.sum ./
RUN go mod download
COPY services/orchestrator ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/orchestrator .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/orchestrator /orchestrator
ENTRYPOINT ["/orchestrator"]
